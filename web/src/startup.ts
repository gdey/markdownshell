// Startup timing. A page notes when it got to each point of starting
// up and, once it has drawn, sends the marks to Go in one request,
// which prints them beside its own when MDSH_STARTUP_TRACE is set (see
// ui/startup.go) and ignores them otherwise. Import this first, so its
// "script" mark is when the page's modules have loaded and start to run.

const marks: [string, number][] = [];

// mark notes that the page got to name now.
export function mark(name: string) {
  marks.push([name, performance.timeOrigin + performance.now()]);
}

mark("script");

// sent says the page has sent its marks, which it does once.
let sent = false;

// drawn notes that the page is ready to use and, after the next frame
// has been drawn, sends its marks with the browser's own: when the page
// had loaded, the first paint and when the DOM was ready.
export function drawn(page: string) {
  if (sent) return;
  sent = true;
  mark("ready");
  requestAnimationFrame(() =>
    setTimeout(() => {
      mark("drawn");
      const at = (name: string, t: number | undefined) => {
        if (t) marks.push([name, performance.timeOrigin + t]);
      };
      const nav = performance.getEntriesByType("navigation")[0] as PerformanceNavigationTiming | undefined;
      at("fetched", nav?.responseEnd);
      at("dom ready", nav?.domContentLoadedEventEnd);
      for (const p of performance.getEntriesByType("paint")) at(p.name, p.startTime);
      const q = new URLSearchParams({ page });
      for (const [name, t] of marks.sort((a, b) => a[1] - b[1])) q.append("m", name + ":" + t.toFixed(1));
      void fetch("/startup-marks?" + q.toString()).catch(() => {});
    }),
  );
}
