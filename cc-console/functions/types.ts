export interface Env {
  DB: D1Database;
  RESEND_API_KEY: string;
  CC_CONTROL_URL: string;
  CC_ADMIN_TOKEN: string;
  CC_WEB_URL: string;
}

export type CF = PagesFunction<Env>;

export function json(data: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

export function err(msg: string, status = 400) {
  return json({ error: msg }, status);
}

export function genId(): string {
  return crypto.randomUUID();
}

export async function getUser(ctx: EventContext<Env, any, any>) {
  const cookie = ctx.request.headers.get("Cookie") || "";
  const match = cookie.match(/session=([^\s;]+)/);
  if (!match) return null;
  const row = await ctx.env.DB.prepare(
    "SELECT u.id, u.email, u.tenant_id, u.tenant_token FROM sessions s JOIN users u ON s.user_id = u.id WHERE s.token = ? AND s.expires_at > ?"
  ).bind(match[1], Date.now()).first<{ id: string; email: string; tenant_id: string | null; tenant_token: string | null }>();
  return row ?? null;
}
