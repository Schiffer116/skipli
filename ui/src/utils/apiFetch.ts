let refreshing: Promise<boolean> | null = null;

async function refresh() {
  const res = await fetch("/api/auth/refresh", { method: "POST" });
  return res.ok;
}

export async function apiFetch(input: string, init?: RequestInit) {
  const res = await fetch(input, init);
  if (res.status !== 401) return res;

  refreshing ??= refresh().finally(() => (refreshing = null));
  return (await refreshing) ? fetch(input, init) : res;
}
