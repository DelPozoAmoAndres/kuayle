import { expect, test } from '@playwright/test';

const user = {
	id: 'a0000000-0000-0000-0000-000000000001',
	email: 'owner@example.test',
	name: 'Owner',
	display_name: 'Owner',
	avatar_url: null,
	is_sysadmin: false
};

function workspace(slug = 'test') {
	return {
		id: 'b0000000-0000-0000-0000-000000000001',
		name: 'Test Workspace',
		slug,
		logo_url: null,
		owner_id: user.id,
		owner: user,
		share_link_min_role: 'admin',
		current_user_role: 'owner',
		created_at: '2026-01-01T00:00:00Z',
		updated_at: '2026-01-01T00:00:00Z'
	};
}

test.beforeEach(async ({ page }) => {
	await page.route('https://raw.githubusercontent.com/**', (route) => route.fulfill({ json: [] }));
});

test('previews, confirms, and imports a workspace archive', async ({ page }) => {
	let importPosted = false;
	let importContentType = '';
	await page.addInitScript(() => {
		(window as typeof window & { __workspaceRefreshes: unknown[] }).__workspaceRefreshes = [];
		window.addEventListener('app:refresh', (event) => {
			(window as typeof window & { __workspaceRefreshes: unknown[] }).__workspaceRefreshes.push(
				(event as CustomEvent).detail
			);
		});
	});
	await page.route('**/api/**', async (route) => {
		const request = route.request();
		const path = new URL(request.url()).pathname;
		if (path === '/api/auth/me') return route.fulfill({ json: user });
		if (path === '/api/workspaces' && request.method() === 'GET') return route.fulfill({ json: [] });
		if (path === '/api/workspaces/import/preview') {
			return route.fulfill({
				json: {
					manifest: {
						format: 'kuayle.workspace',
						version: 1,
						exported_at: '2026-01-01T00:00:00Z',
						source_workspace_id: workspace().id,
						source_workspace_name: 'Restored Workspace',
						source_workspace_slug: 'restored',
						counts: { teams: 2, projects: 1, issues: 12, comments: 5, assets: 3 },
						omitted: ['refresh_tokens'],
						warnings: ['Reconnect GitHub.'],
						requires_reconfiguration: ['GitHub']
					},
					name: 'Restored Workspace',
					slug: 'restored',
					missing_users: []
				}
			});
		}
		if (path === '/api/workspaces/import' && request.method() === 'POST') {
			importPosted = true;
			importContentType = request.headers()['content-type'] ?? '';
			return route.fulfill({
				status: 201,
				json: { id: workspace('restored').id, name: 'Restored Workspace', slug: 'restored', counts: {}, warnings: [], requires_reconfiguration: [] }
			});
		}
		if (path === '/api/workspaces/restored') return route.fulfill({ json: workspace('restored') });
		if (path === '/api/notifications') return route.fulfill({ json: { notifications: [], unread_count: 0 } });
		if (/\/api\/workspaces\/restored\/(statuses|projects|labels|members|views|favorites)$/.test(path)) {
			return route.fulfill({ json: [] });
		}
		return route.fulfill({ status: 404, json: { error: { code: 'UNHANDLED', message: path } } });
	});

	await page.goto('/workspace-setup');
	await expect(page.getByRole('heading', { name: 'Set up your workspace' })).toBeVisible();
	await page.locator('input[type=file]').setInputFiles({ name: 'workspace.kuayle.zip', mimeType: 'application/zip', buffer: Buffer.from('fake archive') });
	await expect(page.getByRole('heading', { name: 'Import workspace' }).last()).toBeVisible();
	await expect(page.getByText('12')).toBeVisible();
	await expect(page.getByText('Reconnect GitHub.')).toBeVisible();
	await page.getByLabel('Type restored to confirm creating this workspace').fill('restored');
	await page.getByRole('button', { name: 'Import workspace' }).last().click();
	await expect.poll(() => importPosted).toBe(true);
	expect(importContentType).toContain('multipart/form-data; boundary=');
	await expect
		.poll(() =>
			page.evaluate(
				() => (window as typeof window & { __workspaceRefreshes: unknown[] }).__workspaceRefreshes
			)
		)
		.toContainEqual({ resources: ['workspace'] });
	await expect(page).toHaveURL(/\/restored\/inbox$/);
});

test('blocks missing users and reports a slug conflict', async ({ page }) => {
	let previews = 0;
	await page.route('**/api/**', async (route) => {
		const request = route.request();
		const path = new URL(request.url()).pathname;
		if (path === '/api/auth/me') return route.fulfill({ json: user });
		if (path === '/api/workspaces' && request.method() === 'GET') return route.fulfill({ json: [] });
		if (path === '/api/workspaces/import/preview') {
			previews++;
			return route.fulfill({
				json: {
					manifest: {
						format: 'kuayle.workspace',
						version: 1,
						exported_at: '2026-01-01T00:00:00Z',
						source_workspace_id: workspace().id,
						source_workspace_name: 'Restored Workspace',
						source_workspace_slug: 'restored',
						counts: { issues: 1 },
						omitted: [],
						warnings: [],
						requires_reconfiguration: []
					},
					name: 'Restored Workspace',
					slug: 'restored',
					missing_users: previews === 1 ? ['missing@example.test'] : []
				}
			});
		}
		if (path === '/api/workspaces/import' && request.method() === 'POST') {
			return route.fulfill({
				status: 409,
				json: { error: { code: 'WORKSPACE_SLUG_TAKEN', message: 'Workspace slug is already taken' } }
			});
		}
		return route.fulfill({ status: 404, json: { error: { code: 'UNHANDLED', message: path } } });
	});

	await page.goto('/workspace-setup');
	const fileInput = page.locator('input[type=file]');
	const archive = { name: 'workspace.kuayle.zip', mimeType: 'application/zip', buffer: Buffer.from('fake archive') };
	await fileInput.setInputFiles(archive);
	await expect(page.getByText('missing@example.test')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Import workspace' }).last()).toBeDisabled();
	await page.getByRole('button', { name: 'Cancel' }).click();

	await fileInput.setInputFiles(archive);
	await page.getByLabel('Type restored to confirm creating this workspace').fill('restored');
	await page.getByRole('button', { name: 'Import workspace' }).last().click();
	await expect(page.locator('.app-toast-shell')).toContainText('Workspace slug is already taken');
});

test('shows export to workspace admins and downloads the archive', async ({ page }) => {
	await page.route('**/api/**', async (route) => {
		const request = route.request();
		const path = new URL(request.url()).pathname;
		if (path === '/api/auth/me') return route.fulfill({ json: user });
		if (path === '/api/workspaces/test') {
			return route.fulfill({ json: { ...workspace(), owner_id: 'a0000000-0000-0000-0000-000000000099', current_user_role: 'admin' } });
		}
		if (path === '/api/workspaces/test/export') {
			return route.fulfill({
				body: Buffer.from('PK fake zip'),
				headers: { 'Content-Type': 'application/zip', 'Content-Disposition': 'attachment; filename="test.kuayle.zip"' }
			});
		}
		if (path === '/api/notifications') return route.fulfill({ json: { notifications: [], unread_count: 0 } });
		if (/\/api\/workspaces\/test\/(statuses|projects|labels|members|views|favorites)$/.test(path)) {
			return route.fulfill({ json: [] });
		}
		return route.fulfill({ status: 404, json: { error: { code: 'UNHANDLED', message: path } } });
	});

	await page.goto('/test/settings');
	await expect(page.getByRole('heading', { name: 'Workspace transfer' })).toBeVisible();
	const download = page.waitForEvent('download');
	await page.getByRole('button', { name: 'Download export' }).click();
	expect((await download).suggestedFilename()).toBe('test.kuayle.zip');
});
