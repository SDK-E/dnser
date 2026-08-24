import { AppShell, Tabs, Title, Text, Group } from '@mantine/core'
import { useEffect, useState } from 'react'
import ProjectsView from './views/ProjectsView'
import LogsView from './views/LogsView'
import DoctorView from './views/DoctorView'
import SettingsView from './views/SettingsView'
import { brand } from './theme'

function Wordmark() {
  const [logoOk, setLogoOk] = useState(false)
  useEffect(() => {
    const img = new Image()
    img.onload = () => setLogoOk(true)
    img.src = '/brand/dnser-logo-dark.png'
  }, [])
  if (logoOk) {
    return <img src="/brand/dnser-logo-dark.png" alt="DNS.er" height={30} />
  }
  return (
    <Title order={3} c={brand.lettersOnDark} fw={700} lh={1}>
      DNS<span style={{ color: brand.accent }}>.</span>er
      <Text span size="xs" c="dimmed" ml="sm">
        local infrastructure
      </Text>
    </Title>
  )
}

export default function App() {
  const [tab, setTab] = useState<string>('projects')
  return (
    <AppShell header={{ height: 64 }} padding="lg">
      <AppShell.Header bg={brand.backgroundDark}>
        <Group h="100%" px="xl" justify="space-between">
          <Wordmark />
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
