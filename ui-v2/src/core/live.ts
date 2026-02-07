import { Accessor, createSignal, onCleanup, onMount } from "solid-js";

export function createNowTicker(intervalMs = 30000): Accessor<number> {
  const [now, setNow] = createSignal(Date.now());
  let timer: number | undefined;

  onMount(() => {
    timer = window.setInterval(() => setNow(Date.now()), intervalMs);
  });

  onCleanup(() => {
    if (timer) window.clearInterval(timer);
  });

  return now;
}

export function useAutoRefresh(refresh: () => void, intervalMs = 30000): void {
  let timer: number | undefined;

  onMount(() => {
    timer = window.setInterval(() => refresh(), intervalMs);
  });

  onCleanup(() => {
    if (timer) window.clearInterval(timer);
  });
}
