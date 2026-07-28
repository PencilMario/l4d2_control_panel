import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type UIEvent,
} from "react";

const BOTTOM_TOLERANCE = 40;

export function useConsoleFollow(outputVersion: unknown) {
  const outputRef = useRef<HTMLPreElement | null>(null);
  const followingRef = useRef(true);
  const [following, setFollowingState] = useState(true);
  const animationFrame = useRef<number | null>(null);

  const setFollowing = useCallback((next: boolean) => {
    followingRef.current = next;
    setFollowingState(next);
  }, []);

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
    setFollowing(true);
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
    scrollToBottom();
  }, [scrollToBottom, setFollowing]);

  const onScroll = useCallback((event: UIEvent<HTMLPreElement>) => {
    const output = event.currentTarget;
    const distance = output.scrollHeight - output.clientHeight - output.scrollTop;
    const atBottom = distance <= BOTTOM_TOLERANCE;
    if (!atBottom && animationFrame.current !== null) {
      cancelAnimationFrame(animationFrame.current);
      animationFrame.current = null;
    }
    setFollowing(atBottom);
  }, [setFollowing]);

  useLayoutEffect(() => {
    if (!followingRef.current) return;
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
    scrollToBottom();
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
    following,
    isFollowing: () => followingRef.current,
  };
}
