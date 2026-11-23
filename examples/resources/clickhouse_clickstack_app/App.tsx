import { useQuery } from '@hdx/app';

// Replaced with the real source id by replace() in resource.tf.
const SOURCE_ID = '__SOURCE_ID__';

export default function App() {
  const errors = useQuery(
    "SELECT ServiceName, count() AS errors FROM otel_logs WHERE SeverityText = 'error' GROUP BY ServiceName ORDER BY errors DESC",
    { sourceId: SOURCE_ID, title: 'Errors by service' },
  );
  if (errors.loading) return <div>Loading…</div>;
  return (
    <ul>
      {errors.data.map(row => (
        <li key={String(row.ServiceName)}>{`${row.ServiceName}: ${row.errors}`}</li>
      ))}
    </ul>
  );
}
