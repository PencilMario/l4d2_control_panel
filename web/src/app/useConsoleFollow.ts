import {
  useCallback,
  useLayoutEffect,
  useRef,
  type UIEvent,
} from "react";

const BOTTOM_TOLERANCE = 4;

export function useConsoleFollow(outputVersion: unknown) {
  const outputRef = useRef<HTMLPreElement | null>(null);
  const following = useRef(true);
  const animationFrame = useRef<number | null>(null);

  const scrollToBottom = useCallback(() => {
    if (animationFrame.current !== null) {
      cancelAnimationFrame(animationFrame.current);
    }
    animationFrame.current = requestAnimationFrame(() => {
      animationFrame.current = null;
      if (outputRef.current) {
        outputRef.current.scrollTop = outputRef.current.scrollHeight;
      }
    });
  }, []);

  const forceFollow = useCallback(() => {
    following.current = true;
    scrollToBottom();
  }, [scrollToBottom]);

  const onScroll = useCallback((event: UIEvent<HTMLPreElement>) => {
    const output = event.currentTarget;
    const distance = output.scrollHeight - output.clientHeight - output.scrollTop;
    const atBottom = distance <= BOTTOM_TOLERANCE;
    if (!atBottom && animationFrame.current !== null) {
      cancelAnimationFrame(animationFrame.current);
      animationFrame.current = null;
    }
    following.current = atBottom;
  }, []);

  useLayoutEffect(() => {
    if (following.current) scrollToBottom();
  }, [outputVersion, scrollToBottom]);

  useLayoutEffect(
    () => () => {
      if (animationFrame.current !== null) {
        cancelAnimationFrame(animationFrame.current);
      }
    },
    [],
  );

  return {
    outputRef,
    forceFollow,
    onScroll,
    isFollowing: () => following.current,
  };
}
