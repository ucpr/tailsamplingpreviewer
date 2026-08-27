import { useMemo } from "react";
import { useAppState } from "../state";
import { useNow } from "../useNow";
import { classifyOutcome, type Outcome } from "./LiveTail";

const BUCKET_MS = 2000;
const BUCKET_COUNT = 60;

type Bucket = Record<Outcome, number>;

function emptyBucket(): Bucket {
  return { keep: 0, drop: 0, live: 0 };
}

export function SpanVolumeChart() {
  const { state } = useAppState();
  const now = useNow(1000);

  const buckets = useMemo(() => {
    const windowEnd = Math.floor(now / BUCKET_MS) * BUCKET_MS;
    const windowStart = windowEnd - BUCKET_COUNT * BUCKET_MS;
    const arr: Bucket[] = Array.from({ length: BUCKET_COUNT }, emptyBucket);
    for (const t of Object.values(state.traces)) {
      const ts = new Date(t.last_seen).getTime();
      if (Number.isNaN(ts)) continue;
      const idx = Math.floor((ts - windowStart) / BUCKET_MS);
      if (idx < 0 || idx >= BUCKET_COUNT) continue;
      arr[idx][classifyOutcome(t)] += t.span_count;
    }
    return arr;
  }, [state.traces, now]);

  const max = Math.max(1, ...buckets.map((b) => b.keep + b.drop + b.live));

  return (
    <div className="span-volume-chart">
      <div className="span-volume-chart__header">
        <span className="span-volume-chart__title">Span Volume</span>
        <div className="span-volume-chart__legend">
          <span className="span-volume-chart__legend-item">
            <span className="span-volume-chart__dot span-volume-chart__dot--keep" />
            KEEP
          </span>
          <span className="span-volume-chart__legend-item">
            <span className="span-volume-chart__dot span-volume-chart__dot--drop" />
            DROP
          </span>
          <span className="span-volume-chart__legend-item">
            <span className="span-volume-chart__dot span-volume-chart__dot--live" />
            LIVE
          </span>
        </div>
      </div>
      <div className="span-volume-chart__bars">
        {buckets.map((b, i) => {
          const total = b.keep + b.drop + b.live;
          const heightPct = (total / max) * 100;
          return (
            <div className="span-volume-chart__bar-col" key={i} title={`${total} spans`}>
              <div className="span-volume-chart__bar" style={{ height: `${heightPct}%` }}>
                <div className="span-volume-chart__seg span-volume-chart__seg--keep" style={{ flex: `${b.keep} 0 0` }} />
                <div className="span-volume-chart__seg span-volume-chart__seg--drop" style={{ flex: `${b.drop} 0 0` }} />
                <div className="span-volume-chart__seg span-volume-chart__seg--live" style={{ flex: `${b.live} 0 0` }} />
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
