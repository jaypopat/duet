import { getSandbox } from "@cloudflare/sandbox";
import { Agent, getAgentByName } from "agents";

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

    url.pathname = restPath;
    return agent.fetch(new Request(url, request));
  },
};

// agent which responds to messages using a lightweight model and can execute sandboxed commands
export class DuetAgent extends Agent<Env, DuetAgentState> {
  override initialState: DuetAgentState = { messages: [] };

  override async onRequest(request: Request): Promise<Response> {
    const url = new URL(request.url);

    if (request.method !== "POST") {
      return Response.json({ error: "method not allowed" }, { status: 405 });
    }

    switch (url.pathname) {
      case "/message": {
        const body = (await request.json().catch(() => ({}))) as any;
        if (!body.text?.trim()) {
          return Response.json({ error: "missing text" }, { status: 400 });
        }

        const now = Date.now();
        const userMsg: DuetMessage = {
          role: "user",
          userId: body.userId?.trim(),
          text: body.text.trim(),
          ts: now,
        };

        const messages = [
          {
            role: "system",
            content: "You are Duet, a concise helpful assistant for a pair-programming session. Answer clearly with short steps. If uncertain, ask one clarifying question.",
          },
          ...this.state.messages.slice(-20).map((m) => ({
            role: m.role === "agent" ? "assistant" : "user",
            content: m.text,
          })),
          {
            role: "user",
            content: userMsg.text,
          },
        ];
        let replyText = "";
        try {
          const result = (await this.env.AI.run("@cf/meta/llama-2-7b-chat-int8", {
            messages,
          }));
          replyText = (result?.response ?? "").toString().trim();
        } catch {
          return Response.json({ error: "ai unavailable" }, { status: 502 });
        }

        if (!replyText) replyText = "I couldn't generate a response.";

        const agentMsg: DuetMessage = {
          role: "agent",
          text: replyText,
          ts: Date.now(),
        };

        const nextMessages = [...this.state.messages, userMsg, agentMsg].slice(-50);
        this.setState({ messages: nextMessages });

        return Response.json({ reply: agentMsg.text, messages: nextMessages });
      }

      case "/sandbox/exec": {

        const body = (await request.json().catch(() => ({}))) as any;
        if (!body.cmd?.trim()) {
          return Response.json({ error: "missing cmd" }, { status: 400 });
        }
        if (!body.sandboxName?.trim()) {
          return Response.json({ error: "missing sandboxName" }, { status: 400 });
        }
        const sandbox = getSandbox(this.env.Sandbox, body.sandboxName?.trim());
        const result = await sandbox.exec(body.cmd.trim());
        return Response.json({ result });
      }

      default:
        return Response.json({ error: "the available endpoints are /message and /sandbox/exec" }, { status: 404 });
    }
  }
}
