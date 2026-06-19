export const AdminPermissions = {
  dashboard: 'admin.dashboard.read',
  ops: 'admin.ops.read',
  users: 'admin.users.manage',
  groups: 'admin.groups.manage',
  subscriptions: 'admin.subscriptions.manage',
  accounts: 'admin.accounts.write'
} as const

export type AdminPermission = typeof AdminPermissions[keyof typeof AdminPermissions]

export const defaultProjectAdminPermissions: AdminPermission[] = [
  AdminPermissions.dashboard,
  AdminPermissions.ops,
  AdminPermissions.users,
  AdminPermissions.groups,
  AdminPermissions.subscriptions,
  AdminPermissions.accounts
]
