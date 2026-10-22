// fetch rejects with a TypeError when a request never got an answer; each
// browser words it differently.
export const isUnreachable = (error: unknown) =>
  error instanceof TypeError &&
  /fetch|network|load failed/i.test(error.message);
