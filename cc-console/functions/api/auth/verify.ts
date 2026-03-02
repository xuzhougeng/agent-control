import { json, err, genId, type CF } from "../../types";

export const onRequestPost: CF = async (ctx) => {
  try {
  const body = await ctx.request.json<{ email?: string; code?: string }>();
  const email = body.email?.trim().toLowerCase();
  const code = body.code?.trim();
  if (!email || !code) return err("email and code required");

  const now = Date.now();

  const row = await ctx.env.DB.prepare(
    "SELECT id FROM verification_codes WHERE email = ? AND code = ? AND expires_at > ? AND used = 0 ORDER BY expires_at DESC LIMIT 1"
  ).bind(email, code, now).first<{ id: string }>();

  if (!row) return err("Invalid or expired code", 401);

  await ctx.env.DB.prepare("UPDATE verification_codes SET used = 1 WHERE id = ?").bind(row.id).run();

  let user = await ctx.env.DB.prepare("SELECT id, tenant_id, tenant_token FROM users WHERE email = ?")
    .bind(email).first<{ id: string; tenant_id: string | null; tenant_token: string | null }>();

  if (!user) {
    const userId = genId();
    let tenantId: string | null = null;
    let tenantToken: string | null = null;

    const slot = await ctx.env.DB.prepare(
      "SELECT id, tenant_id, tenant_token FROM tenant_pool WHERE assigned_to IS NULL LIMIT 1"
    ).first<{ id: number; tenant_id: string; tenant_token: string }>();

    if (!slot) {
      return err("Tenant pool exhausted. Please deploy your own instance: https://github.com/xuzhougeng/agent-control", 503);
    }

    tenantId = slot.tenant_id;
    tenantToken = slot.tenant_token;

    await ctx.env.DB.prepare(
      "INSERT INTO users (id, email, tenant_id, tenant_token, created_at) VALUES (?, ?, ?, ?, ?)"
    ).bind(userId, email, tenantId, tenantToken, now).run();

    await ctx.env.DB.prepare(
      "UPDATE tenant_pool SET assigned_to = ? WHERE id = ?"
    ).bind(userId, slot.id).run();
    user = { id: userId, tenant_id: tenantId, tenant_token: tenantToken };
  }

  const sessionToken = genId();
  const expiresAt = now + 7 * 86400_000; // 7 days

  await ctx.env.DB.prepare(
    "INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)"
  ).bind(sessionToken, user.id, expiresAt).run();

  return json(
    { ok: true, user: { id: user.id, email, tenant_id: user.tenant_id, tenant_token: user.tenant_token }, web_url: ctx.env.CC_WEB_URL || "" },
    200,
    { "Set-Cookie": `session=${sessionToken}; Path=/; HttpOnly; SameSite=Lax; Secure; Max-Age=604800` }
  );
  } catch (e: any) {
    console.error("verify error:", e?.message || e);
    return err("Internal error: " + (e?.message || "unknown"), 500);
  }
};
