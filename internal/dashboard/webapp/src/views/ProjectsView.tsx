import { Badge, Loader, Alert } from '@mantine/core'
import { DataTable } from 'mantine-datatable'
import { useQuery } from '@tanstack/react-query'
import { fetchStatus, ProjectRow } from '../api'

const phaseColor: Record<string, string> = {
  ready: 'green',
  running: 'teal',
  starting: 'yellow',
  stopping: 'orange',
  stopped: 'gray',
  backoff: 'red',
}

export default function ProjectsView() {
  const status = useQuery({ queryKey: ['status'], queryFn: fetchStatus, refetchInterval: 4000 })

  if (status.isPending) return <Loader />
  if (status.isError) return <Alert color="red" title="API error">{String(status.error)}</Alert>

  const rows: ProjectRow[] = status.data.projects
  return (
    <DataTable
      withTableBorder
      borderRadius="md"
      records={rows}
      idAccessor="name"
      columns={[
        { accessor: 'name', title: 'Project' },
        { accessor: 'domain', title: 'Domain' },
        { accessor: 'port', title: 'Port' },
        { accessor: 'availability', title: 'Tier' },
        {
          accessor: 'phase',
          title: 'State',
          render: (p) => (
            <Badge color={phaseColor[p.phase] ?? 'gray'} variant="light">
              {p.phase}
            </Badge>
          ),
        },
        { accessor: 'pid', title: 'PID', render: (p) => (p.pid ? String(p.pid) : '—') },
      ]}
      noRecordsText="No linked projects yet — run dnser link"
    />
  )
}
