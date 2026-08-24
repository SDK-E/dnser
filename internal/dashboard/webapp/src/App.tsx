import { AppShell, Tabs, Title, Text, Group } from '@mantine/core'
import { useState } from 'react'
import ProjectsView from './views/ProjectsView'
import LogsView from './views/LogsView'
import DoctorView from './views/DoctorView'
import SettingsView from './views/SettingsView'

export default function App() {
  const [tab, setTab] = useState<string>('projects')
  return (
    <AppShell header={{ height: 64 }} padding="lg">
      <AppShell.Header bg="dark.9">
        <Group h="100%" px="xl" justify="space-between">
          <Group gap="sm">
            <Title order={3} c="moss.5" ff="-apple-system, BlinkMacSystemFont, sans-serif" fw={700}>
              DNS.er
            </Title>
            <Text size="sm" c="dimmed">
              local infrastructure
            </Text>
          </Group>
        </Group>
      </AppShell.Header>
      <AppShell.Main>
        <Tabs value={tab} onChange={(v) => setTab(v ?? 'projects')}>
          <Tabs.List mb="md">
            <Tabs.Tab value="projects">Projects</Tabs.Tab>
            <Tabs.Tab value="logs">Logs</Tabs.Tab>
            <Tabs.Tab value="doctor">Doctor</Tabs.Tab>
            <Tabs.Tab value="settings">Settings</Tabs.Tab>
          </Tabs.List>
          {tab === 'projects' && <ProjectsView />}
          {tab === 'logs' && <LogsView />}
          {tab === 'doctor' && <DoctorView />}
          {tab === 'settings' && <SettingsView />}
        </Tabs>
      </AppShell.Main>
    </AppShell>
  )
}
