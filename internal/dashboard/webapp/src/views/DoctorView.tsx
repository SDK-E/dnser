import { Alert, Badge, Code, Loader, Stack } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { fetchDoctor, DoctorIssue } from '../api'

const kindColor: Record<string, string> = {
  resolver_drift: 'orange',
  dead_resolver: 'red',
  interrupted_plan: 'yellow',
  stray_listener: 'red',
  shadowed_suffix: 'grape',
  fallback_dns: 'blue',
}

export default function DoctorView() {
  const doctor = useQuery({ queryKey: ['doctor'], queryFn: fetchDoctor, refetchInterval: 15000 })

  if (doctor.isPending) return <Loader />
  if (doctor.isError) return <Alert color="red">{String(doctor.error)}</Alert>

  const issues: DoctorIssue[] = doctor.data.issues
  if (issues.length === 0) return <Alert color="green" title="clean">No issues found.</Alert>

  return (
    <Stack gap="sm">
      {issues.map((i, idx) => (
        <Alert key={idx} color={kindColor[i.kind] ?? 'gray'} title={i.kind}>
          <Stack gap={4}>
            <Badge variant="light" w="fit-content">
              evidence
            </Badge>
            <Code>{i.evidence}</Code>
            <Badge variant="light" color="moss" w="fit-content">
              fix
            </Badge>
            <Code>{i.fix}</Code>
          </Stack>
        </Alert>
      ))}
    </Stack>
  )
}
