import { useEffect, useRef, useState } from 'react'
import { Select, ScrollArea, Code, Stack } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { fetchStatus, fetchLogs } from '../api'

export default function LogsView() {
  const status = useQuery({ queryKey: ['status'], queryFn: fetchStatus })
  const [project, setProject] = useState<string | null>(null)
  const viewport = useRef<HTMLDivElement>(null)

  const projects = (status.data?.projects ?? []).map((p) => p.name)
  const active = project ?? projects[0] ?? null

  const logs = useQuery({
    queryKey: ['logs', active],
    queryFn: () => fetchLogs(active!),
    enabled: !!active,
    refetchInterval: 2500,
  })

  useEffect(() => {
    if (viewport.current) {
      viewport.current.scrollTop = viewport.current.scrollHeight
    }
  }, [logs.data])

  return (
    <Stack gap="sm">
      <Select
        placeholder="pick a project"
        data={projects}
        value={active}
        onChange={setProject}
        w={280}
      />
      <ScrollArea h="60vh" type="auto" viewportRef={viewport}>
        <Code block style={{ background: '#082003', color: '#a4eba6', fontSize: 12.5 }}>
          {(logs.data?.lines ?? []).map((l) => l.line).join('\n')}
        </Code>
      </ScrollArea>
    </Stack>
  )
}
