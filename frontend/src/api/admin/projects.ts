import { apiClient } from '../client'

export type ProjectRole = 'super_admin' | 'admin' | 'user'
export type AssignableProjectRole = 'admin' | 'user'
export type ProjectStatus = 'active' | 'disabled'
export type ProjectMemberStatus = 'active' | 'disabled'
export type ProjectProfileMode = 'restricted' | 'unrestricted'
export type ProjectResourceType = 'users' | 'groups' | 'accounts' | 'subscriptions' | 'api_keys'

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
  status: ProjectMemberStatus
  created_at?: string
  updated_at?: string
}

export interface ProjectProfile {
  id: number
  project_id: number
  name: string
  description?: string | null
  mode: ProjectProfileMode
  is_active: boolean
  created_at?: string
  updated_at?: string
}

export interface ProjectProfileBindings {
  profile_id: number
  user_ids: number[]
  group_ids: number[]
  account_ids: number[]
  subscription_ids: number[]
  api_key_ids: number[]
}

export interface ProjectResourceUserCandidate {
  id: number
  email: string
  username: string
  notes: string
  status: string
}

export interface ProjectResourceGroupCandidate {
  id: number
  project_id: number
  name: string
  description: string
  platform: string
  status: string
}

export interface ProjectResourceAccountCandidate {
  id: number
  project_id: number
  name: string
  notes: string
  platform: string
  type: string
  status: string
  email: string
}

export interface ProjectResourceSubscriptionCandidate {
  id: number
  user_id: number
  group_id: number
  user_email: string
  group_name: string
  status: string
  notes: string
}

export interface ProjectResourceAPIKeyCandidate {
  id: number
  user_id: number
  project_id: number
  name: string
  key_prefix: string
  user_email: string
  status: string
}

export interface ProjectResourceSearchResult {
  users: ProjectResourceUserCandidate[]
  groups: ProjectResourceGroupCandidate[]
  accounts: ProjectResourceAccountCandidate[]
  subscriptions: ProjectResourceSubscriptionCandidate[]
  api_keys: ProjectResourceAPIKeyCandidate[]
}

export interface CreateProjectRequest {
  name: string
  slug: string
  description?: string | null
  profile_mode?: ProjectProfileMode
  user_ids?: number[]
  group_ids?: number[]
  account_ids?: number[]
  subscription_ids?: number[]
  api_key_ids?: number[]
}

export interface UpdateProjectRequest {
  name?: string
  description?: string | null
  status?: ProjectStatus
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
  payload: { role: AssignableProjectRole; is_owner?: boolean; status?: ProjectMemberStatus }
): Promise<ProjectMember> {
  const { data } = await apiClient.put<ProjectMember>(
    `/admin/projects/${projectId}/members/${userId}`,
    payload
  )
  return data
}

export async function listProfiles(projectId: number): Promise<ProjectProfile[]> {
  const { data } = await apiClient.get<ProjectProfile[]>(`/admin/projects/${projectId}/profiles`)
  return data
}

export async function createProfile(
  projectId: number,
  payload: { name: string; description?: string | null; mode: ProjectProfileMode }
): Promise<ProjectProfile> {
  const { data } = await apiClient.post<ProjectProfile>(`/admin/projects/${projectId}/profiles`, payload)
  return data
}

export async function updateProfile(
  projectId: number,
  profileId: number,
  payload: { name?: string; description?: string | null; mode?: ProjectProfileMode }
): Promise<ProjectProfile> {
  const { data } = await apiClient.put<ProjectProfile>(`/admin/projects/${projectId}/profiles/${profileId}`, payload)
  return data
}

export async function deleteProfile(projectId: number, profileId: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/projects/${projectId}/profiles/${profileId}`)
  return data
}

export async function activateProfile(projectId: number, profileId: number): Promise<ProjectProfile> {
  const { data } = await apiClient.post<ProjectProfile>(`/admin/projects/${projectId}/profiles/${profileId}/activate`)
  return data
}

export async function getProfileBindings(projectId: number, profileId: number): Promise<ProjectProfileBindings> {
  const { data } = await apiClient.get<ProjectProfileBindings>(`/admin/projects/${projectId}/profiles/${profileId}/bindings`)
  return data
}

export async function setProfileBindings(
  projectId: number,
  profileId: number,
  payload: ProjectProfileBindings
): Promise<ProjectProfileBindings> {
  const { data } = await apiClient.put<ProjectProfileBindings>(
    `/admin/projects/${projectId}/profiles/${profileId}/bindings`,
    payload
  )
  return data
}

export async function searchBindableResources(
  projectId: number,
  query: string,
  limit = 20
): Promise<ProjectResourceSearchResult> {
  const { data } = await apiClient.get<ProjectResourceSearchResult>(`/admin/projects/${projectId}/resources/search`, {
    params: { q: query, limit }
  })
  return data
}

export async function searchGlobalBindableResources(
  query: string,
  limit = 20
): Promise<ProjectResourceSearchResult> {
  const { data } = await apiClient.get<ProjectResourceSearchResult>('/admin/projects/resources/search', {
    params: { q: query, limit }
  })
  return data
}

export async function removeMember(projectId: number, userId: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    `/admin/projects/${projectId}/members/${userId}`
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
  listProfiles,
  createProfile,
  updateProfile,
  deleteProfile,
  activateProfile,
  getProfileBindings,
  setProfileBindings,
  searchBindableResources,
  searchGlobalBindableResources
}
