export default defineEventHandler(async (event) => {
  const { secure } = await requireUserSession(event);
  if (!secure) throw createError({ statusCode: 401, message: "Unauthorized" });

  throw createError({
    statusCode: 500,
    message: "Not implemented",
  });
});
