import { json, err, getUser, type CF } from "../../types";

export const onRequestGet: CF = async (ctx) => {
  const user = await getUser(ctx);
  if (!user) return err("Not authenticated", 401);
  return json({ user, web_url: ctx.env.CC_WEB_URL || "" });
};
