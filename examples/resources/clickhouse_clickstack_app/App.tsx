import { useQuery } from '@hdx/app';

// resource.tf replaces the quoted placeholder below with a jsonencode()d
// object, which is a valid JavaScript literal. The cast keeps this file
// type-checking before the replacement. Keep the placeholder out of the rest
// of the file: replace() swaps every occurrence.
const CONFIG = '__CONFIG__' as unknown as { sourceId: string; heading: string };

export default function App() {
  const errors = useQuery(
    "SELECT ServiceName, count() AS errors FROM otel_logs WHERE SeverityText = 'error' GROUP BY ServiceName ORDER BY errors DESC",
    { sourceId: CONFIG.sourceId, title: 'Errors by service' },
  );
  if (errors.loading) return <div>Loading…</div>;
  return (
    <div>
      <h2>{CONFIG.heading}</h2>
      <ul>
        {errors.data.map(row => (
          <li key={String(row.ServiceName)}>{`${row.ServiceName}: ${row.errors}`}</li>
        ))}
      </ul>
    </div>
  );
}
