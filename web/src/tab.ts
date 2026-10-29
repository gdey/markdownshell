// A tab's page runs inside its window's tab bar page, in an iframe whose
// URL names the tab. Every call it makes to Go says which tab it is in
// the x-mdsh-tab header, so the call acts on that tab, whichever window
// holds it. Import this before anything that calls Go.

export const tabID = new URLSearchParams(location.search).get("tab") ?? "";

const fetchGo = window.fetch.bind(window);
window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
  const url = new URL(input instanceof Request ? input.url : String(input), location.href);
  if (tabID && url.pathname.startsWith("/wails/")) {
    const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined));
    headers.set("x-mdsh-tab", tabID);
    init = { ...init, headers };
  }
  return fetchGo(input, init);
};
