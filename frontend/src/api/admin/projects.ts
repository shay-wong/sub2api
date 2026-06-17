import { apiClient } from '../client'

export type ProjectRole = 'super_admin' | 'admin' | 'user'
export type AssignableProjectRole = 'admin' | 'user'
export type ProjectStatus = 'active' | 'disabled'

export interface AdminProject {
  id: number
  name: string
  slug: string
  description?: string | null
  role?: ProjectRole
  is_owner?: boolean
}

export interface ProjectMember {
  project_id: number
  user_id: number
  email: string
  username: string
  role: AssignableProjectRole
  is_owner: boolean
  status: 'active' | 'disabled'
  created_at?: string
  updated_at?: string
}

export interface CreateProjectRequest {
  name: string
  slug: string
  description?: string | null
}

export interface UpdateProjectRequest {
  name?: string
  description?: string | null
  status?: ProjectStatus
}

export interface MoveProjectResourcesRequest {
  account_ids?: number[]
  api_key_ids?: number[]
  group_ids?: number[]
  move_usage_history?: boolean
}

export interface MoveProjectResourcesResult {
  accounts_moved: number
  api_keys_moved: number
  groups_moved: number
  account_group_bindings_removed: number
  api_key_group_bindings_cleared: number
  group_fallbacks_cleared: number
  group_model_routing_cleared: number
  project_members_added: number
  usage_logs_moved: number
  ops_error_logs_moved: number
}

export async function list(): Promise<AdminProject[]> {
  const { data } = await apiClient.get<AdminProject[]>('/admin/projects')
  return data
}

export async function create(payload: CreateProjectRequest): Promise<AdminProject> {
  const { data } = await apiClient.post<AdminProject>('/admin/projects', payload)
  return data
}

export async function update(id: number, payload: UpdateProjectRequest): Promise<AdminProject> {
  const { data } = await apiClient.put<AdminProject>(`/admin/projects/${id}`, payload)
  return data
}

export async function listMembers(projectId: number): Promise<ProjectMember[]> {
  const { data } = await apiClient.get<ProjectMember[]>(`/admin/projects/${projectId}/members`)
  return data
}

export async function setMember(
  projectId: number,
  userId: number,
  payload: { role: AssignableProjectRole; is_owner?: boolean }
): Promise<ProjectMember> {
  const { data } = await apiClient.put<ProjectMember>(
    `/admin/projects/${projectId}/members/${userId}`,
    payload
  )
  return data
}

export async function removeMember(projectId: number, userId: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    `/admin/projects/${projectId}/members/${userId}`
  )
  return data
}

export async function moveResources(
  projectId: number,
  payload: MoveProjectResourcesRequest
): Promise<MoveProjectResourcesResult> {
  const { data } = await apiClient.post<MoveProjectResourcesResult>(
    `/admin/projects/${projectId}/resources/move`,
    payload
  )
  return data
}

export default {
  list,
  create,
  update,
  listMembers,
  setMember,
  removeMember,
  moveResources
}
