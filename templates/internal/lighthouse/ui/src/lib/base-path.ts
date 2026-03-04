const normalizeBase = (value: string | undefined) => {
  const trimmed = (value || "/lighthouse/").trim();
  const withLeadingSlash = trimmed.startsWith("/") ? trimmed : `/${trimmed}`;
  const withoutTrailingSlash = withLeadingSlash.replace(/\/+$/, "");
  return withoutTrailingSlash || "/lighthouse";
};

export const lighthouseBasePath = normalizeBase(import.meta.env.BASE_URL);

export const lighthousePath = (path: string) => {
  const normalized = path.startsWith("/") ? path : `/${path}`;
  return `${lighthouseBasePath}${normalized}`;
};

export const lighthouseWSURL = (path: string) => {
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  return `${scheme}://${window.location.host}${lighthousePath(path)}`;
};
