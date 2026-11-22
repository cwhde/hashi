// 13.12 gate checks for the Public pages editor and the public pages (07 §7, §8.1, ADR 0004), called from the main
// journey once there are entries to arrange. Kept apart so the steps read as one story: drag with a mouse, with a finger
// (hold, then drag; a swipe scrolls instead), Alt+arrows, Undo; then the public pages and a service page at phone and
// desktop width, and a badge as an SVG.
import { expect, type Page } from '@playwright/test';

type Layout = {
	version: number;
	groups: { id?: string; name: string; entries: { key: string; name: string }[] }[];
};

async function layout(page: Page, base: string): Promise<Layout> {
	return (await page.request.get(`${base}/api/public-layout`)).json();
}
const groupOf = (l: Layout, key: string) =>
	l.groups.find((g) => g.entries.some((e) => e.key === key))?.name;

type Point = { x: number; y: number };
type Locator = ReturnType<Page['locator']>;

/** A pointer that can press, move and release: the mouse, or a finger through the browser's own touch input
 * (pointer events with pointerType "touch"). */
type Pointer = {
	down(p: Point): Promise<void>;
	move(p: Point): Promise<void>;
	up(): Promise<void>;
};

const mouse = (page: Page): Pointer => ({
	down: async (p) => {
		await page.mouse.move(p.x, p.y);
		await page.mouse.down();
	},
	move: (p) => page.mouse.move(p.x, p.y),
	up: () => page.mouse.up()
});

async function finger(page: Page): Promise<Pointer> {
	const cdp = await page.context().newCDPSession(page);
	await cdp.send('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 1 });
	const touch = (type: 'touchStart' | 'touchMove' | 'touchEnd', touchPoints: Point[]) =>
		cdp.send('Input.dispatchTouchEvent', { type, touchPoints });
	return {
		down: (p) => touch('touchStart', [p]).then(() => {}),
		move: (p) => touch('touchMove', [p]).then(() => {}),
		up: async () => {
			await touch('touchEnd', []);
			await cdp.send('Emulation.setTouchEmulationEnabled', { enabled: false });
			await cdp.detach();
		}
	};
}

const centre = async (sel: Locator): Promise<Point> => {
	const b = (await sel.boundingBox())!;
	return { x: b.x + b.width / 2, y: b.y + b.height / 2 };
};

/** Moves a pressed pointer to the target in small steps. The layout changes while the dragged entry passes other
 * groups, so the target is measured again after each leg until the entry has arrived (or the target is gone). */
async function dragInto(p: Pointer, from: Point, target: Locator, arrived: () => Promise<boolean>) {
	let at = from;
	for (let leg = 0; leg < 6 && !(await arrived()); leg++) {
		if (!(await target.count())) break;
		const to = await centre(target);
		for (let i = 1; i <= 8; i++)
			await p.move({ x: at.x + ((to.x - at.x) * i) / 8, y: at.y + ((to.y - at.y) * i) / 8 });
		at = to;
	}
}

export async function publicPagesEditor(page: Page, base: string) {
	await page.setViewportSize({ width: 1440, height: 900 });
	// The old address leads to the editor.
	await page.goto(`${base}/app-display`);
	await expect(page).toHaveURL(/\/public-pages$/);
	await expect(page.getByRole('heading', { name: 'Public pages', level: 1 })).toBeVisible();
	const outline = page.getByRole('list', { name: 'Layout' });
	await expect(outline).toBeVisible();

	// A new group, named at once; both changes are saved.
	await page.getByRole('button', { name: 'New group', exact: true }).click();
	await page.getByRole('textbox', { name: 'Group name' }).fill('E2E group');
	await page.keyboard.press('Enter');
	await expect
		.poll(async () => (await layout(page, base)).groups.map((g) => g.name))
		.toContain('E2E group');
	const group = outline.getByRole('listitem', { name: 'E2E group' });

	// Mouse: drag the first entry into the empty group. The order changes while the pointer moves, before the release.
	const row = outline.locator('[data-sort-kind="item"][data-sort-id]').first();
	const key = (await row.getAttribute('data-sort-id'))!;
	const from = groupOf(await layout(page, base), key)!;
	const name = (await layout(page, base)).groups
		.flatMap((g) => g.entries)
		.find((e) => e.key === key)!.name;
	const inGroup = group.locator(`[data-sort-id="${key}"]`);
	const arrived = async () => (await inGroup.count()) === 1;
	const m = mouse(page);
	const start = await centre(row.locator('span.truncate'));
	await m.down(start);
	await dragInto(m, start, group.getByText('Empty'), arrived);
	await expect(inGroup).toHaveCount(1); // still held
	await m.up();
	await expect(page.getByText(`${name} moved to E2E group. Saved.`)).toBeVisible();
	await expect.poll(async () => groupOf(await layout(page, base), key)).toBe('E2E group');
	await page.waitForTimeout(300); // the row settles into its place
	await page.screenshot({ path: 'test-results/13-12-editor.png' });

	// Undo puts it back and saves that.
	await page.getByRole('button', { name: 'Undo' }).click();
	await expect.poll(async () => groupOf(await layout(page, base), key)).toBe(from);

	// A fast release: Chrome hands a held-back move and the release over in one go. Every item settles into its place and
	// keeps no lifted look (on prod the frame queued by that move put the lift back for good). Undo restores the order.
	const c = await centre(
		outline.locator('[data-sort-kind="item"][data-sort-id]').nth(1).locator('span.truncate')
	);
	await m.down(c);
	for (let i = 1; i <= 6; i++) await m.move({ x: c.x + i * 12, y: c.y + i * 10 });
	await page.evaluate(
		([x, y]) => {
			const o = { pointerId: 1, pointerType: 'mouse', isPrimary: true, clientX: x, clientY: y };
			window.dispatchEvent(new PointerEvent('pointermove', o));
			window.dispatchEvent(new PointerEvent('pointerup', o));
		},
		[c.x + 90, c.y + 80]
	);
	await m.up();
	await page.waitForTimeout(400);
	const lifted = () =>
		page.evaluate(() =>
			[...document.querySelectorAll<HTMLElement>('[data-sort-id]')]
				.filter((e) => e.style.transform)
				.map((e) => e.textContent?.trim())
		);
	expect(await lifted()).toEqual([]);
	const undo = page.getByRole('button', { name: 'Undo' });
	if (await undo.isEnabled()) await undo.click();
	await expect.poll(async () => groupOf(await layout(page, base), key)).toBe(from);

	// Keyboard: Alt+Down on the grip moves it one place (into the next group when it was last); it keeps the focus.
	// By the key: two entries may share a name (a web and a TCP service of one app).
	const mine = outline.locator(`[data-sort-id="${key}"]`);
	const grip = mine.getByRole('button', { name: `Move ${name}: Alt and the arrow keys` });
	const at = async () => {
		const l = await layout(page, base);
		const g = l.groups.find((g) => g.entries.some((e) => e.key === key))!;
		return `${g.name}:${g.entries.findIndex((e) => e.key === key)}`;
	};
	const was = await at();
	await grip.focus();
	await page.keyboard.press('Alt+ArrowDown');
	await expect.poll(at).not.toBe(was);
	await expect(grip).toBeFocused();
	await expect(page.locator('[aria-live="polite"]').last()).toContainText(name);
	await page.keyboard.press('Alt+ArrowUp');
	await expect.poll(at).toBe(was);

	// Touch: a quick swipe on a row scrolls and moves nothing; holding still first, then dragging, moves it. In a tab of
	// its own: touch emulation on this page left its virtual passkey unable to answer later in the journey.
	const tab = await page.context().newPage();
	await tab.setViewportSize({ width: 1440, height: 900 });
	await tab.goto(`${base}/public-pages`);
	const tabOutline = tab.getByRole('list', { name: 'Layout' });
	const tabRow = tabOutline.locator(`[data-sort-id="${key}"]`).locator('span.truncate');
	const tabGroup = tabOutline.getByRole('listitem', { name: 'E2E group' });
	const tabIn = tabGroup.locator(`[data-sort-id="${key}"]`);
	await expect(tabRow).toBeVisible();
	const version = (await layout(page, base)).version;
	const s1 = await centre(tabRow);
	const swipe = await finger(tab);
	await swipe.down(s1);
	for (const dy of [20, 40, 60, 80]) await swipe.move({ x: s1.x, y: s1.y + dy });
	await swipe.up();
	await tab.waitForTimeout(500);
	expect((await layout(page, base)).version, 'a swipe saved a change').toBe(version);
	await expect(tabIn).toHaveCount(0);

	const s2 = await centre(tabRow);
	const hold = await finger(tab);
	await hold.down(s2);
	await tab.waitForTimeout(500); // held still past the hold time
	await hold.move({ x: s2.x, y: s2.y + 2 });
	await dragInto(hold, s2, tabGroup.getByText('Empty'), async () => (await tabIn.count()) === 1);
	await hold.up();
	await expect.poll(async () => groupOf(await layout(page, base), key)).toBe('E2E group');
	await tab.close();
	await page.reload(); // the tab saved newer versions
	await expect(inGroup).toHaveCount(1);

	// Put it back by Move to, empty the group and delete it, so later steps see the layout as before.
	await mine.getByRole('checkbox', { name: `Select ${name}` }).click();
	await page.getByRole('button', { name: 'Move to' }).click();
	await page.getByRole('menuitem', { name: from, exact: true }).click();
	await expect.poll(async () => groupOf(await layout(page, base), key)).toBe(from);
	await group.getByRole('button', { name: 'Delete E2E group' }).click();
	await expect
		.poll(async () => (await layout(page, base)).groups.map((g) => g.name))
		.not.toContain('E2E group');

	// Phone: the outline only, the page behind a Preview button.
	await page.setViewportSize({ width: 390, height: 844 });
	await page.getByRole('button', { name: 'Preview' }).click();
	await expect(page.getByRole('dialog').getByRole('radio', { name: 'Status page' })).toBeVisible();
	await page.keyboard.press('Escape');
	expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
		true
	);
	await page.setViewportSize({ width: 1440, height: 900 });
}

/** Both public pages and a service page fit a phone and a desktop; a badge is served as an SVG. */
export async function publicPagesSeen(page: Page, appsURL: string, statusURL: string) {
	const st = await (await page.request.get(`${statusURL}/api/public/status`)).json();
	const slug: string = st.groups.flatMap((g: { entries: { slug: string }[] }) => g.entries)[0].slug;
	for (const [w, h] of [
		[390, 844],
		[1440, 900]
	] as const) {
		await page.setViewportSize({ width: w, height: h });
		for (const [url, file] of [
			[appsURL, 'apps'],
			[statusURL, 'status'],
			[`${statusURL}/s/${slug}`, 'service']
		] as const) {
			await page.goto(url);
			await expect(page.locator('body')).not.toContainText('500');
			await expect(page.getByRole('heading').first()).toBeVisible();
			expect(
				await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
				`${file} at ${w} px is wider than the screen`
			).toBe(true);
			await page.screenshot({ path: `test-results/13-12-${file}-${w}.png`, fullPage: true });
		}
	}
	// The status page's view too.
	await page.goto(`${statusURL}/?view=hour`);
	await expect(page.getByRole('radio', { name: 'Last hour' })).toBeChecked();
	await page.getByRole('radio', { name: '90 days' }).click();
	await expect(page).toHaveURL(new RegExp(`${statusURL}/?$`));
	// The service page's range is in the address: a shared link opens the same range, picking one updates it.
	await page.goto(`${statusURL}/s/${slug}?range=7d`);
	await expect(page.getByRole('radio', { name: '7d' })).toBeChecked();
	await page.getByRole('radio', { name: '30d' }).click();
	await expect(page).toHaveURL(/\?range=30d$/);
	await page.getByRole('radio', { name: '24h' }).click();
	await expect(page).toHaveURL(new RegExp(`/s/${slug}$`));
	// The charts read by keyboard too (bug 175): Home goes to the first moment and names it.
	const chart = page.getByRole('slider', { name: /^Response time, latest/ });
	await chart.focus();
	await page.keyboard.press('Home');
	await expect(chart).toHaveAttribute('aria-valuenow', '0');
	await expect(chart).toHaveAttribute('aria-valuetext', /\d/);
	// A service page for nothing shown answers 404 and still says so (bug 172).
	const gone = await page.request.get(`${statusURL}/s/no-such-service`);
	expect(gone.status()).toBe(404);
	expect(await gone.text()).toContain('<title>');
	const svc = await (await page.request.get(`${statusURL}/api/public/status/${slug}`)).json();
	const badge = await page.request.get(`${statusURL}/badge/${slug}/${svc.badges[0]}.svg`);
	expect(badge.status()).toBe(200);
	expect(badge.headers()['content-type']).toBe('image/svg+xml');
	expect(await badge.text()).toContain('<svg');
	// Reduced motion: a looping animation runs once and stays still. At 0.01 ms and looping, spinners and loading
	// placeholders jumped to a new frame on every repaint.
	await page.emulateMedia({ reducedMotion: 'reduce' });
	const loops = await page.evaluate(() =>
		['animate-spin', 'animate-pulse'].map((c) => {
			const el = document.body.appendChild(document.createElement('div'));
			el.className = c;
			const n = getComputedStyle(el).animationIterationCount;
			el.remove();
			return n;
		})
	);
	expect(loops).toEqual(['1', '1']);
	await page.emulateMedia({ reducedMotion: null });
}
