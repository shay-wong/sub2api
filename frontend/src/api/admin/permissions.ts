import { apiClient } from '../client'

export interface OperatorPermissionSubject {
  id: number
  email: string
  username: string
  role: 'operator' | 'user'
  status: 'active' | 'disabled'
  group_ids: number[]
  created_at?: string
  updated_at?: string
}

export interface UpdateOperatorPermissionRequest {
  role: 'operator' | 'user'
  group_ids: number[]
}

export async function listOperators(): Promise<OperatorPermissionSubject[]> {
  const { data } = await apiClient.get<OperatorPermissionSubject[]>('/admin/permissions/operators')
  return data
}

export async function updateOperator(
  id: number,
  payload: UpdateOperatorPermissionRequest
): Promise<OperatorPermissionSubject> {
  const { data } = await apiClient.put<OperatorPermissionSubject>(
    `/admin/permissions/operators/${id}`,
    payload
  )
  return data
}

export default {
  listOperators,
  updateOperator
}
