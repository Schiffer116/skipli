import { useEffect, useRef, type RefObject } from "react";

// Calls onOutsidePointerDown when a pointer goes down anywhere outside the
// element the ref is attached to, while the calling component is mounted
// (e.g. to close an open "create" form when the user clicks away from it).
export default function useOutsidePointerDown(
  ref: RefObject<Element | null>,
  onOutsidePointerDown: () => void,
) {
  const callback = useRef(onOutsidePointerDown);

  useEffect(() => {
    callback.current = onOutsidePointerDown;
  });

  useEffect(() => {
    const handlePointerDown = (e: PointerEvent) => {
      const element = ref.current;
      if (
        !element ||
        (e.target instanceof Node && element.contains(e.target))
      ) {
        return;
      }
      callback.current();
    };

    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, [ref]);
}
