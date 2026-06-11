// Mirrors the Go JSON shapes in apps/dashboard-api/internal/model/model.go.
// Keep in sync by hand (no shared package since the backend is Go).

export interface ThermalZone {
  type: string
  tempC: number
}

export interface DiskUsage {
  mount: string
  device: string
  totalBytes: number
  usedBytes: number
  freeBytes: number
  usedPct: number
}

export interface HostStats {
  cpuPct: number
  mem: { totalBytes: number; usedBytes: number; usedPct: number }
  disks: DiskUsage[]
  uptimeSec: number
  cpuTempC: number | null
  thermalZones: ThermalZone[]
  collectedAt: string
}

export interface ContainerInfo {
  id: string
  name: string
  image: string
  state: string
  status: string
  cpuPct: number | null
  memUsedBytes: number | null
  memLimitBytes: number | null
}

export interface K3sNode {
  name: string
  ready: boolean
  cpuMilli: number | null
  cpuCapacityMilli: number | null
  memUsedBytes: number | null
  memCapacityBytes: number | null
}

export interface K3sPod {
  namespace: string
  name: string
  phase: string
  ready: string
  restarts: number
  cpuMilli: number | null
  memUsedBytes: number | null
}

export interface K3sData {
  nodes: K3sNode[]
  pods: K3sPod[]
}
