import { Stack, Title, List, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { fetchStatus } from '../api'

export default function SettingsView() {
  const status = useQuery({ queryKey: ['status'], queryFn: fetchStatus })
  const dnsPort = status.data?.dns_port ?? '—'
  return (
    <Stack gap="md" maw={560}>
      <Title order={4}>Runtime</Title>
      <List spacing="xs">
        <List.Item>
          DNS listener: <Text span c="moss.5" inherit>127.0.0.1:{dnsPort}</Text>
        </List.Item>
        <List.Item>Daemon: {status.data?.daemon.running ? 'running' : 'not running'}</List.Item>
      </List>
      <Text size="sm" c="dimmed">
        Effective per-project configuration is available via `dnser explain` and will surface
        here as the daemon API grows. Dashboard is loopback-only; keep the ?token= URL private.
      </Text>
    </Stack>
  )
}
