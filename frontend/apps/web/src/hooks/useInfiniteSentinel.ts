import { useEffect, useRef } from "react";

// useInfiniteSentinel wires an IntersectionObserver on a bottom sentinel div to
// auto-load the next page when it scrolls into view. Returns the ref to attach.
// Without IntersectionObserver (older WebViews, jsdom) the list simply stops at
// the loaded pages instead of throwing.
export function useInfiniteSentinel(hasNext: boolean, isFetching: boolean, fetchNext: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el || typeof IntersectionObserver === "undefined") return;
    const obs = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && hasNext && !isFetching) fetchNext();
      },
      { rootMargin: "300px" },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [hasNext, isFetching, fetchNext]);
  return ref;
}
