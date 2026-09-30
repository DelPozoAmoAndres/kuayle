import type { Role } from './roles';

const ROLE_PERMISSIONS: Record<Role, string[]> = {
	owner: [
		'workspace:manage',
		'issue:create',
		'issue:read',
		'issue:update',
		'issue:delete',
		'issue:delete_own',
		'project:manage',
		'label:manage',
		'member:invite',
		'view:manage'
	],
	admin: [
		'workspace:manage',
		'issue:create',
		'issue:read',
		'issue:update',
		'issue:delete',
		'issue:delete_own',
		'project:manage',
		'label:manage',
		'member:invite',
		'view:manage'
	],
	member: [
		'issue:create',
		'issue:read',
		'issue:update',
		'issue:delete_own',
		'project:manage',
		'label:manage',
		'view:manage'
	],
	guest: ['issue:read']
};

export function hasPermission(role: Role, permission: string): boolean {
	return ROLE_PERMISSIONS[role]?.includes(permission) ?? false;
}
