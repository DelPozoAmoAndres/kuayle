import { expect, test } from '@playwright/test';

test('shows workspace analytics and opens the explorer', async ({ page }) => {
	const issueListQueries: URLSearchParams[] = [];
	const pageErrors: Error[] = [];
	page.on('pageerror', (error) => {
		pageErrors.push(error);
	});
	await page.route('https://raw.githubusercontent.com/carbogninalberto/kuayle/main/UI/static/releases.json', (route) =>
		route.fulfill({ json: [] })
	);
	await page.route('**/api/**', async (route) => {
		const requestUrl = new URL(route.request().url());
		const path = requestUrl.pathname;
		if (path === '/api/auth/me') {
			return route.fulfill({
				json: {
					id: '00000000-0000-0000-0000-000000000001',
					email: 'test@example.com',
					name: 'Test User',
					display_name: 'Test User',
					avatar_url: null
				}
			});
		}
		if (path === '/api/preferences') {
			return route.fulfill({
				json: {
					font_size: 'default',
					pointer_cursors: true,
					theme_mode: 'dark',
					light_theme: 'light',
					dark_theme: 'dark',
					workflow_sort_mode: 'default',
					workflow_sort_order: ['backlog', 'unstarted', 'started', 'completed', 'cancelled']
				}
			});
		}
		if (path === '/api/workspaces/test') {
			return route.fulfill({
				json: {
					id: '00000000-0000-0000-0000-000000000002',
					name: 'Test Workspace',
					slug: 'test',
					logo_url: null,
					created_at: '2026-01-01T00:00:00Z',
					updated_at: '2026-01-01T00:00:00Z'
				}
			});
		}
		if (path === '/api/workspaces') {
			return route.fulfill({
				json: [
					{
						id: '00000000-0000-0000-0000-000000000002',
						name: 'Test Workspace',
						slug: 'test',
						logo_url: null,
						created_at: '2026-01-01T00:00:00Z',
						updated_at: '2026-01-01T00:00:00Z'
					}
				]
			});
		}
		if (path === '/api/workspaces/test/statuses') {
			return route.fulfill({
				json: [
					{
						id: '00000000-0000-0000-0000-000000000003',
						workspace_id: '00000000-0000-0000-0000-000000000002',
						name: 'In progress',
						slug: 'in-progress',
						category: 'started',
						color: '#6366f1',
						position: 2,
						is_default: false,
						created_at: '2026-01-01T00:00:00Z',
						updated_at: '2026-01-01T00:00:00Z'
					}
				]
			});
		}
		if (
			path === '/api/workspaces/test/projects' ||
			path === '/api/workspaces/test/labels' ||
			path === '/api/workspaces/test/members' ||
			path === '/api/workspaces/test/views'
		) {
			return route.fulfill({ json: [] });
		}
		if (path === '/api/notifications') {
			return route.fulfill({ json: { notifications: [], unread_count: 0 } });
		}
		if (path === '/api/workspaces/test/analytics/overview') {
			return route.fulfill({
				json: {
					total_issues: 42,
					open_issues: 18,
					completed_issues: 24,
					overdue_issues: 3,
					total_projects: 4,
					total_members: 8,
					started_issues: 6,
					unassigned_issues: 2,
					completion_rate: 57.14,
					avg_lead_time_hours: 48,
					avg_cycle_time_hours: 24
				}
			});
		}
		if (path === '/api/workspaces/test/analytics/distribution') {
			return route.fulfill({
				json: {
					by_status: [
						{
							status_id: '00000000-0000-0000-0000-000000000004',
							name: 'Backlog',
							color: null,
							category: 'backlog',
							count: 5
						},
						{
							status_id: '00000000-0000-0000-0000-000000000003',
							name: 'In progress',
							color: '#6366f1',
							category: 'started',
							count: 18
						}
					],
					by_priority: [{ priority: 2, count: 12 }]
				}
			});
		}
		if (path === '/api/workspaces/test/analytics/burnup') {
			return route.fulfill({
				json: {
					interval: 'week',
					from: '2026-04-14',
					to: '2026-07-12',
					points: [{ date: '2026-07-06', created: 5, completed: 3, total_created: 42, total_completed: 24, scope: 18 }]
				}
			});
		}
		if (path === '/api/workspaces/test/analytics/insights') {
			const requestedSlice = requestUrl.searchParams.get('slice') ?? 'none';
			const group =
				requestedSlice === 'status_type'
					? { key: 'started', label: 'Started' }
					: {
							key: '00000000-0000-0000-0000-000000000003',
							label: 'In progress',
							color: '#6366f1'
						};
			return route.fulfill({
				json: {
					measure: 'issue_count',
					slice: requestedSlice,
					segment: 'none',
					unit: 'issues',
					total_count: 42,
					aggregate: 42,
					groups: [
						{
							...group,
							count: 18,
							value: 18,
							segments: []
						}
					],
					points: []
				}
			});
		}
		if (path === '/api/workspaces/test/issues') {
			issueListQueries.push(requestUrl.searchParams);
			return route.fulfill({ json: { data: [], total_count: 0, page: 1, has_more: false } });
		}
		return route.fulfill({ status: 404, json: { error: { message: `Unhandled ${path}` } } });
	});

	await page.goto('/test/insights');
	await expect(page.getByRole('heading', { name: 'Insights' })).toBeVisible();
	await expect(page.getByText('Total issues')).toBeVisible();
	await expect(page.getByText('42', { exact: true })).toBeVisible();
	await expect(page.getByText('Completion rate')).toBeVisible();
	await expect(page.getByText('57%')).toBeVisible();
	await page.getByRole('tab', { name: 'Explore' }).click();
	await expect(page).toHaveURL(/tab=explore/);
	await expect(page.getByLabel('Measure')).toContainText('Issue count');
	await page.getByLabel('Group by').click();
	await page.getByRole('option', { name: 'Status', exact: true }).click();
	await expect(page.getByLabel('Group by')).toContainText('Status');
	await page.getByLabel('Date range').click();
	await expect(page.getByRole('button', { name: '90 days' })).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(page.getByText('In progress', { exact: true })).toBeVisible();

	await page.goto('/test/insights?tab=explore');
	await expect.poll(() => issueListQueries.length).toBeGreaterThan(0);
	const drillDownQuery = issueListQueries.at(-1)!;
	expect(drillDownQuery.has('assignee')).toBe(false);
	expect(drillDownQuery.has('creator')).toBe(false);

	await page.goto('/test/insights?tab=explore&slice=status_type');
	const statusTypeQueryCount = issueListQueries.length;
	await page.getByText('Started', { exact: true }).click();
	await expect(page).toHaveURL('/test/my-issues?status_type=started');
	await expect.poll(() => issueListQueries.length).toBeGreaterThan(statusTypeQueryCount);
	const statusTypeQuery = issueListQueries.at(-1)!;
	expect(statusTypeQuery.get('status_type')).toBe('started');
	expect(statusTypeQuery.has('assignee')).toBe(false);
	expect(statusTypeQuery.has('creator')).toBe(false);

	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto('/test/insights?tab=explore');
	await expect(page.getByRole('tab', { name: 'Explore' })).toBeVisible();
	await expect(page.getByLabel('Measure')).toBeVisible();
	await expect.poll(() => pageErrors.map((error) => error.message)).toEqual([]);
});
