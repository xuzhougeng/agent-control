import { json, err, genId, type Env, type CF } from "../../types";

export const onRequestPost: CF = async (ctx) => {
  const body = await ctx.request.json<{ email?: string }>();
  const email = body.email?.trim().toLowerCase();
  if (!email || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    return err("Invalid email");
  }

  const now = Date.now();
  const recent = await ctx.env.DB.prepare(
    "SELECT COUNT(*) as cnt FROM verification_codes WHERE email = ? AND expires_at > ?"
  ).bind(email, now - 3600_000).first<{ cnt: number }>();
  if (recent && recent.cnt >= 5) {
    return err("Too many attempts (max 5 per hour). Please try again in 1 hour.", 429);
  }

  const code = String(Math.floor(100000 + Math.random() * 900000));
  const expiresAt = now + 600_000; // 10 min

  await ctx.env.DB.prepare(
    "INSERT INTO verification_codes (id, email, code, expires_at) VALUES (?, ?, ?, ?)"
  ).bind(genId(), email, code, expiresAt).run();

  const res = await fetch("https://api.resend.com/emails", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${ctx.env.RESEND_API_KEY}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      from: "Agent Control <noreply@cc-remote.app>",
      to: [email],
      subject: `Your verification code: ${code}`,
      html: `
        <div style="font-family:sans-serif;max-width:480px;margin:0 auto;padding:40px 24px;">
          <h2 style="color:#1d2430;margin-bottom:8px;">Agent Control</h2>
          <p style="color:#59616c;">Your verification code is:</p>
          <div style="font-size:36px;font-weight:700;letter-spacing:8px;color:#315f72;
                      background:#f6efe6;border-radius:12px;padding:20px;text-align:center;
                      margin:24px 0;">${code}</div>
          <p style="color:#8a847a;font-size:14px;">This code expires in 10 minutes. If you did not request this, ignore this email.</p>
        </div>`,
    }),
  });

  if (!res.ok) {
    const detail = await res.text();
    console.error("Resend error:", detail);
    return err("Failed to send email", 500);
  }

  return json({ ok: true });
};
