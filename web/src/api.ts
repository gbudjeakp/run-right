import axios from 'axios'
import type { Job, MachineType } from './types'

const api = axios.create({
  baseURL: '/api/v1',
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true, // send the HttpOnly session cookie automatically
})

export const login = (apiKey: string): Promise<void> =>
  api.post('/auth', { api_key: apiKey }).then(() => undefined)

export const logout = (): Promise<void> =>
  api.post('/auth/logout').then(() => undefined)

export const fetchJobs = (): Promise<Job[]> =>
  api.get<Job[]>('/jobs').then((r) => r.data ?? [])

export const fetchJob = (id: number): Promise<Job> =>
  api.get<Job>(`/jobs/${id}`).then((r) => r.data)

export const fetchCatalog = (provider?: string): Promise<MachineType[]> =>
  api.get<MachineType[]>('/catalog', { params: provider ? { provider } : {} }).then((r) => r.data ?? [])
