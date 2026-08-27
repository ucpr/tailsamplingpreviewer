import { useAppState } from "../state";
import { formatCount, formatPercent } from "../format";

export function StatisticsPanel() {
  const { state } = useAppState();
  const stats = state.statistics;

  const matches = Object.entries(stats?.policy_matches ?? {}).sort((a, b) =>
    a[0].localeCompare(b[0]),
  );

  return (
    <section className="panel">
      <h2 className="panel__title">Statistics</h2>

      <dl className="stat-grid">
        <div>
          <dt>Observed</dt>
          <dd>{formatCount(stats?.observed ?? 0)}</dd>
        </div>
        <div>
          <dt>KEEP</dt>
          <dd className="text-keep">{formatCount(stats?.keep ?? 0)}</dd>
        </div>
        <div>
          <dt>DROP</dt>
          <dd className="text-drop">{formatCount(stats?.drop ?? 0)}</dd>
        </div>
        <div>
          <dt>Sampling Rate</dt>
          <dd>{formatPercent(stats?.sampling_rate ?? 0)}</dd>
        </div>
      </dl>

      <h3 className="panel__subtitle">Policy Matches</h3>
      {matches.length === 0 ? (
        <p className="app-header__empty">No matches yet.</p>
      ) : (
        <table className="policy-match-table">
          <thead>
            <tr>
              <th>Policy</th>
              <th>Matches</th>
            </tr>
          </thead>
          <tbody>
            {matches.map(([name, count]) => (
              <tr key={name}>
                <td>{name}</td>
                <td>{formatCount(count)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
