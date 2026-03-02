import { json, err, genId, getUser, type CF } from "../../types";

export const onRequestPost: CF = async (ctx) => {
  const user = await getUser(ctx);
  if (!user) return err("Not authenticated", 401);

  const dbUser = await ctx.env.DB.prepare(
    "SELECT tenant_token FROM users WHERE id = ?"
  ).bind(user.id).first<{ tenant_token: string | null }>();

  if (!dbUser?.tenant_token) return err("No tenant provisioned. Contact support.", 400);

  const body = await ctx.request.json<{ role?: string }>().catch(() => ({}));
  const role = (body as any).role || "owner";

  const res = await fetch(`${ctx.env.CC_CONTROL_URL}/tenant/tokens`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${dbUser.tenant_token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ role }),
  });

  if (!res.ok) {
    const detail = await res.text();
    console.error("cc-control error:", detail);
    return err("Failed to generate tokens", 502);
  }

  const data = await res.json<{
    tenant_id: string;
    ui: { token: string; token_id: string; role: string };
    agent: { token: string; token_id: string };
  }>();

  await ctx.env.DB.prepare(
    "INSERT INTO issued_tokens (id, user_id, ui_token_id, agent_token_id, label, created_at) VALUES (?, ?, ?, ?, ?, ?)"
  ).bind(genId(), user.id, data.ui.token_id, data.agent.token_id, role, Date.now()).run();

  return json({
    tenant_id: data.tenant_id,
    ui_token: data.ui.token,
    ui_role: data.ui.role,
    agent_token: data.agent.token,
  });
};

export const onRequestGet: CF = async (ctx) => {
  const user = await getUser(ctx);
  if (!user) return err("Not authenticated", 401);

  const rows = await ctx.env.DB.prepare(
    "SELECT id, ui_token_id, agent_token_id, label, created_at FROM issued_tokens WHERE user_id = ? ORDER BY created_at DESC LIMIT 20"
  ).bind(user.id).all();

  return json({ tokens: rows.results });
};
