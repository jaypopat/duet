import { getSandbox, Sandbox as SandboxDO } from "@cloudflare/sandbox";
import { Agent, getAgentByName } from "agents";
import { z } from "zod";

const MessageRequestSchema = z.object({
  text: z.string().min(1, "Text cannot be empty"),
  userId: z.string().optional(),
});

const SandboxExecRequestSchema = z.object({
  cmd: z.string().min(1, "Command cannot be empty"),
});

type DuetMessage = {
  role: "user" | "agent";
  userId?: string;
  text: string;
  ts: number;
};

type DuetAgentState = {
  messages: DuetMessage[];
};

// worker which routes requests to DuetAgent instances based on room ID
export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/health") return new Response("ok");

    const match = url.pathname.match(/^\/api\/rooms\/([^/]+)(\/.*)?$/);
    if (!match) return new Response("not found", { status: 404 });

    const [, roomId, restPath = "/"] = match;

    if (restPath !== "/message" && restPath !== "/sandbox/exec") {
      return new Response("not found", { status: 404 });
    }
    if (!roomId) {
      return new Response("missing room ID", { status: 400 });
    }
    const agent = await getAgentByName(env.DUET_AGENT, roomId);

    const agentUrl = new URL(request.url);
    agentUrl.pathname = restPath;

    const headers = new Headers(request.headers);
    headers.set("x-room-id", roomId);

    return agent.fetch(new Request(agentUrl, {
      method: request.method,
      headers,
      body: request.body,
    }));
  }
};
type AIMessage = {
  role: "system" | "user" | "assistant";
  content: string;
}
// agent which responds to messages using a lightweight model and can execute sandboxed commands
export class DuetAgent extends Agent<Env, DuetAgentState> {
  override initialState: DuetAgentState = { messages: [] };

  async runAI(messages: AIMessage[]): Promise<string> {
    const result = await this.env.AI.run("@cf/meta/llama-3-8b-instruct", { messages });
    return result.response?.trim() || "";
  }

  override async onRequest(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const roomId = request.headers.get("x-room-id") || "default";


    if (request.method !== "POST") {
      return Response.json({ error: "method not allowed" }, { status: 405 });
    }

    switch (url.pathname) {
      case "/message": {
        const rawBody = await request.json();
        const parseResult = MessageRequestSchema.safeParse(rawBody);

        if (!parseResult.success) {
          return Response.json(
            {
              error: "invalid request",
              details: z.flattenError(parseResult.error),
            },
            { status: 400 }
          );
        }

        const data = parseResult.data;

        const userMsg: DuetMessage = { role: "user", userId: data.userId?.trim(), text: data.text.trim(), ts: Date.now() };

        const aiMessages: AIMessage[] = [
          {
            role: "system",
            content:
              "You are Duet, a concise pair-programming assistant. " +
              "You can run commands in a sandbox using <run>command</run> tags. " +
              "When asked to perform an action, briefly explain what you will do and wrap the exact shell command(s) in <run> tags. " +
              "Do NOT include predicted output in your response - just provide the explanation and command. "
          },
          ...this.state.messages.slice(-10).map<AIMessage>((m) => ({
            role: m.role === "agent" ? "assistant" : "user",
            content: m.text,
          })),
          { role: "user", content: userMsg.text },
        ];


        let text = await this.runAI(aiMessages);
        const matches = Array.from(text.matchAll(/<run>([\s\S]*?)<\/run>/g));

        for (const match of matches) {
          const cmd = match[1]?.trim();
          if (!cmd) continue;

          try {
            const sandbox = getSandbox(this.env.Sandbox, `sandbox-${roomId}`);
            const { stderr, stdout } = await sandbox.exec(cmd);

            const summary =
              stdout.slice(0, 500) ||
              stderr.slice(0, 500) ||
              "[no output]";

            text += `\n\nOutput (${cmd}):\n${summary}`;
          } catch (e) {
            const msg = e instanceof Error ? e.message : String(e);
            text += `\n\nError (${cmd}):\n${msg}`;
          }
        }


        const agentMsg: DuetMessage = {
          role: "agent",
          text: text,
          ts: Date.now(),
        };

        const nextMessages = [...this.state.messages, userMsg, agentMsg].slice(-50);
        this.setState({ messages: nextMessages });

        return Response.json({ reply: agentMsg.text, messages: nextMessages });
      }

      case "/sandbox/exec": {
        const rawBody = await request.json();
        const parseResult = SandboxExecRequestSchema.safeParse(rawBody);

        if (!parseResult.success) {
          return Response.json(
            {
              error: "invalid request",
              details: z.flattenError(parseResult.error).fieldErrors,
            },
            { status: 400 }
          );
        }

        const data = parseResult.data;
        // each room gets a sandbox too
        const sandboxName = `sandbox-${roomId}`;

        try {
          const sandbox = getSandbox(this.env.Sandbox, sandboxName);
          const result = await sandbox.exec(data.cmd);


          return Response.json({ result, sandboxName });
        } catch (error) {
          return Response.json({
            error: `sandbox execution failed: ${error instanceof Error ? error.message : 'unknown error'}`
          }, { status: 500 });
        }
      }

      default:
        return Response.json({ error: "the available endpoints are /message and /sandbox/exec" }, { status: 404 });
    }
  }
}
export { SandboxDO as Sandbox };
