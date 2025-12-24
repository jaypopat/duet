import { getSandbox } from "@cloudflare/sandbox";

// biome-ignore lint/performance/noBarrelFile: Required by cf workers
export { Sandbox } from "@cloudflare/sandbox";

import { Agent, getAgentByName } from "agents";
import { z } from "zod";

const MessageRequestSchema = z.object({
  text: z.string().min(1, "Text cannot be empty"),
  userId: z.string().optional(),
});

const SandboxExecRequestSchema = z.object({
  cmd: z.string().min(1, "Command cannot be empty"),
});

interface DuetMessage {
  role: "user" | "agent";
  userId?: string;
  text: string;
  ts: number;
}

interface DuetAgentState {
  messages: DuetMessage[];
}
const REGEX_ROOM_ID_PATH = /^\/api\/rooms\/([^/]+)(\/.*)?$/;

// worker which routes requests to DuetAgent instances based on room ID
export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/health") {
      return new Response("ok");
    }
    const match = url.pathname.match(REGEX_ROOM_ID_PATH);
    if (!match) {
      return new Response("not found - room ID is required", { status: 404 });
    }

    const [, roomId, restPath] = match;

    if (!roomId) {
      return new Response("not found - room ID is required", { status: 404 });
    }

    if (restPath !== "/message" && restPath !== "/sandbox/exec") {
      return new Response(
        "not found - only /message and /sandbox/exec endpoints are supported",
        { status: 404 }
      );
    }

    const agent = await getAgentByName(env.DUET_AGENT, roomId);

    const agentUrl = new URL(request.url);
    agentUrl.pathname = restPath;

    const headers = new Headers(request.headers);
    headers.set("x-room-id", roomId);

    return agent.fetch(
      new Request(agentUrl, {
        method: request.method,
        headers,
        body: request.body,
      })
    );
  },
};
interface AIMessage {
  role: "system" | "user" | "assistant";
  content: string;
}
// agent which responds to messages using a lightweight model and can execute sandboxed commands
export class DuetAgent extends Agent<Env, DuetAgentState> {
  override initialState: DuetAgentState = { messages: [] };

  override async onRequest(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const roomId = request.headers.get("x-room-id") || "default";

    if (request.method !== "POST") {
      return Response.json({ error: "method not allowed - only POST is supported" }, { status: 405 });
    }

    const rawBody = await request.json();

    switch (url.pathname) {
      case "/message":
        return this.handleMessage(roomId, rawBody);

      case "/sandbox/exec":
        return this.handleSandboxExec(roomId, rawBody);

      default:
        return Response.json(
          { error: "the available endpoints are /message and /sandbox/exec" },
          { status: 404 }
        );
    }
  }

  private async runAI(messages: AIMessage[]): Promise<string> {
    const result = await this.env.AI.run("@cf/meta/llama-3-8b-instruct", {
      messages,
    });
    return result.response?.trim() || "";
  }

  private async handleMessage(
    roomId: string,
    rawBody: unknown
  ): Promise<Response> {
    const parseResult = MessageRequestSchema.safeParse(rawBody);

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

    const userMsg: DuetMessage = {
      role: "user",
      userId: data.userId?.trim(),
      text: data.text.trim(),
      ts: Date.now(),
    };

    const aiMessages: AIMessage[] = [
      {
        role: "system",
        content:
          "You are Duet, a concise pair-programming assistant. " +
          "You can run commands in a sandbox using <run>command</run> tags. " +
          "When asked to perform an action, briefly explain what you will do and wrap the exact shell command(s) in <run> tags. " +
          "Do NOT include predicted output in your response - just provide the explanation and command.",
      },
      ...this.state.messages.slice(-10).map<AIMessage>((m) => ({
        role: m.role === "agent" ? "assistant" : "user",
        content: m.text,
      })),
      { role: "user", content: userMsg.text },
    ];

    const text = await this.runAI(aiMessages);
    const textWithOutputs = await this.executeCommands(text, roomId);

    const agentMsg: DuetMessage = {
      role: "agent",
      text: textWithOutputs,
      ts: Date.now(),
    };

    const nextMessages = [...this.state.messages, userMsg, agentMsg].slice(-50);
    this.setState({ messages: nextMessages });

    return Response.json({ reply: agentMsg.text, messages: nextMessages });
  }

  private async executeCommands(text: string, roomId: string): Promise<string> {
    const matches = Array.from(text.matchAll(/<run>([\s\S]*?)<\/run>/g));
    let result = text;

    for (const match of matches) {
      const cmd = match[1]?.trim();
      if (!cmd) {
        continue;
      }

      try {
        const sandbox = getSandbox(this.env.Sandbox, `sandbox-${roomId}`);
        const { stderr, stdout } = await sandbox.exec(cmd);

        const summary =
          stdout.slice(0, 500) || stderr.slice(0, 500) || "[no output]";
        result += `\n\nOutput (${cmd}):\n${summary}`;
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        result += `\n\nError (${cmd}):\n${msg}`;
      }
    }
    return result.replace(/<run>[\s\S]*?<\/run>/g, "").trim() || "";
  }

  private async handleSandboxExec(
    roomId: string,
    rawBody: unknown
  ): Promise<Response> {
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
    const sandboxName = `sandbox-${roomId}`;

    try {
      const sandbox = getSandbox(this.env.Sandbox, sandboxName);
      const result = await sandbox.exec(data.cmd);

      return Response.json({ result, sandboxName });
    } catch (error) {
      return Response.json(
        {
          error: `sandbox execution failed: ${error instanceof Error ? error.message : "unknown error"}`,
        },
        { status: 500 }
      );
    }
  }
}
