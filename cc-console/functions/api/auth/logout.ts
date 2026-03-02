import { json, type CF } from "../../types";

export const onRequestPost: CF = async (ctx) => {
  const cookie = ctx.request.headers.get("Cookie") || "";
  const match = cookie.match(/session=([^\s;]+)/);
  if (match) {
    await ctx.env.DB.prepare("DELETE FROM sessions WHERE token = ?").bind(match[1]).run();
  }
  return json({ ok: true }, 200, {
    "Set-Cookie": "session=; Path=/; HttpOnly; SameSite=Lax; Secure; Max-Age=0",
  });
};
