import { useEffect, useMemo, useState } from "react";
import { formatStackwalkFrame, parseStackwalk, type StackwalkFrame } from "./stackwalk";

type Props = {
  value: string;
  status?: "loading" | "ready" | "error";
  onRetry?: () => void;
};

export function StackwalkView({ value, status = value ? "ready" : "loading", onRetry }: Props) {
  const threads = useMemo(() => parseStackwalk(value), [value]);
  const defaultThread = threads.find((thread) => thread.crashed) || threads[0];
  const [selectedThreadID, setSelectedThreadID] = useState(defaultThread?.id || "");

  useEffect(() => {
    setSelectedThreadID(defaultThread?.id || "");
  }, [defaultThread?.id, value]);

  const activeThread = threads.find((thread) => thread.id === selectedThreadID) || defaultThread;
  if (status === "loading" && !value) {
    return <div className="crash-stackwalk-region" role="region" aria-label="调用栈"><div className="crash-inline-empty">正在读取调用栈…</div></div>;
  }
  if (status === "error") {
    return (
      <div className="crash-stackwalk-region" role="region" aria-label="调用栈">
        <div className="crash-inline-empty crash-stackwalk-error">
          <span>读取调用失败</span>
          {onRetry ? <button type="button" className="crash-diagnostic-action" aria-label="重新读取 Stackwalk" onClick={onRetry}>重新读取 Stackwalk</button> : null}
        </div>
      </div>
    );
  }
  if (!activeThread) {
    return <div className="crash-stackwalk-region" role="region" aria-label="调用栈"><div className="crash-inline-empty">暂无可用调用栈</div></div>;
  }

  return <div className="crash-stackwalk-region" role="region" aria-label="调用栈"><div className="crash-stackwalk-view">
    {threads.length > 1 ? (
      <label className="crash-stackwalk-thread-select">
        <span>线程</span>
        <select aria-label="Stackwalk线程" value={activeThread.id} onChange={(event) => setSelectedThreadID(event.target.value)}>
          {threads.map((thread) => <option key={thread.id} value={thread.id}>{`Thread ${thread.id}${thread.crashed ? "（崩溃线程）" : ""}`}</option>)}
        </select>
      </label>
    ) : null}
    <ol className="crash-stackwalk-list" aria-label="调用栈">
      {activeThread.frames.map((frame) => <StackwalkFrameView key={`${frame.index}-${frame.raw}`} frame={frame} />)}
    </ol>
  </div></div>;
}

function StackwalkFrameView({ frame }: { frame: StackwalkFrame }) {
  return (
    <li className="crash-stackwalk-entry crash-stackwalk-frame" aria-label={`#${frame.index} ${formatStackwalkFrame(frame)}`}>
      <span className="crash-stackwalk-index">#{frame.index}</span>
      <div className="crash-stackwalk-location">
        <strong>{frame.module}</strong>
        {frame.symbol ? <span className="crash-stackwalk-symbol">{frame.symbol}</span> : null}
        {frame.offset ? <code>{frame.offset}</code> : null}
        {frame.foundBy ? <small>来源：{frame.foundBy}</small> : null}
      </div>
    </li>
  );
}
