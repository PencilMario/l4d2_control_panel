import { FileWarning } from "lucide-react";
import { parseStackwalk, type StackwalkEntry } from "./stackwalk";

type Props = {
  value: string;
};

export function StackwalkView({ value }: Props) {
  const entries = parseStackwalk(value);
  if (!entries.length) return <div className="crash-inline-empty">暂无 stackwalk 输出</div>;

  return (
    <ol className="crash-stackwalk-list" aria-label="调用栈">
      {entries.map((entry, position) => <StackwalkEntryView key={`${position}-${entry.raw}`} entry={entry} />)}
    </ol>
  );
}

function StackwalkEntryView({ entry }: { entry: StackwalkEntry }) {
  if (entry.kind === "log") {
    return (
      <li className="crash-stackwalk-entry crash-stackwalk-log">
        <span className="crash-stackwalk-kind"><FileWarning /></span>
        <code title={entry.raw}>{entry.raw}</code>
      </li>
    );
  }

  return (
    <li className="crash-stackwalk-entry crash-stackwalk-frame">
      <span className="crash-stackwalk-index">#{entry.index}</span>
      <div className="crash-stackwalk-location">
        <strong>{entry.module}</strong>
        {entry.symbol ? <span className="crash-stackwalk-symbol">{entry.symbol}</span> : null}
        {entry.offset ? <code>{entry.offset}</code> : null}
        {entry.foundBy ? <small>来源：{entry.foundBy}</small> : null}
      </div>
    </li>
  );
}
