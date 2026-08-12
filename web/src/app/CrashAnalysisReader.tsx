import { useEffect, useRef } from "react";
import { ArrowLeft, Bot } from "lucide-react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

type Props = {
  analysis: string;
  instanceName: string;
  reportID: string;
  onBack: () => void;
};

const shortID = (value: string) => `${value.slice(0, 10)}…${value.slice(-8)}`;

export function CrashAnalysisReader({ analysis, instanceName, reportID, onBack }: Props) {
  const backButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    backButton.current?.focus();
  }, []);

  return (
    <section className="crash-analysis-reader" aria-label="AI 分析阅读器">
      <header className="crash-analysis-reader-head">
        <button ref={backButton} className="crash-analysis-back" type="button" onClick={onBack}>
          <ArrowLeft />
          <span>返回崩溃详情</span>
        </button>
        <div className="crash-analysis-reader-title">
          <span><Bot /></span>
          <div>
            <p className="eyebrow">AI CRASH ANALYSIS</p>
            <p className="crash-analysis-reader-label">AI 崩溃分析</p>
            <p>{instanceName} <code>{shortID(reportID)}</code></p>
          </div>
        </div>
      </header>
      <article className="crash-analysis-markdown">
        <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ img: () => null }}>{analysis}</ReactMarkdown>
      </article>
    </section>
  );
}
