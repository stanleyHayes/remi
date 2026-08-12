import AxeBuilder from "@axe-core/playwright";
import { expect, test, type APIRequestContext, type BrowserContext, type Page } from "@playwright/test";

const apiURL = process.env.MEMBER_E2E_API_URL || "http://127.0.0.1:8088";
const memberEmail = process.env.MEMBER_E2E_EMAIL || "ama.member@remi.church";
const memberCode = process.env.MEMBER_E2E_CODE || "260811";
let cachedTokens: { accessToken: string; refreshToken: string } | null = null;

async function authenticate(request: APIRequestContext, context: BrowserContext) {
  if (!cachedTokens) {
    const challengeResponse = await request.post(`${apiURL}/api/member-auth/otp/request`, { data: { identifier: memberEmail } });
    expect(challengeResponse.ok()).toBeTruthy();
    const challenge = await challengeResponse.json();
    const verifiedResponse = await request.post(`${apiURL}/api/member-auth/otp/verify`, { data: { challengeId: challenge.challengeId, code: memberCode, deviceName: "Member production E2E" } });
    expect(verifiedResponse.ok()).toBeTruthy();
    cachedTokens = await verifiedResponse.json();
  }
  const tokens = cachedTokens!;
  await context.addCookies([
    { name: "remi_member_access", value: tokens.accessToken, url: "http://127.0.0.1:3012", httpOnly: true, sameSite: "Lax" },
    { name: "remi_member_refresh", value: tokens.refreshToken, url: "http://127.0.0.1:3012", httpOnly: true, sameSite: "Lax" },
  ]);
}

async function expectAccessible(page: Page, name: string) {
  const result = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"]).analyze();
  const blocking = result.violations.filter(item => ["critical", "serious"].includes(item.impact || ""));
  expect(blocking, `${name}: ${blocking.map(item => `${item.id} (${item.nodes.length})`).join(", ")}`).toEqual([]);
}

test("public recovery, privacy and sign-in surfaces are branded and accessible", async ({ page }) => {
  for (const route of ["/sign-in", "/privacy", "/terms", "/offline", "/missing-member-route"]) {
    await page.goto(route);
    await expect(page.locator("body")).toBeVisible();
    await expectAccessible(page, route);
  }
  await expect(page.locator("select")).toHaveCount(0);
});

test("authenticated member routes remain self-contained, accessible and mobile-safe", async ({ page, request, context }) => {
  await authenticate(request, context);
  const routes: Array<[string, RegExp]> = [
    ["/", /Good |Your week|Welcome/i], ["/participation", /journey|registrations/i], ["/serving", /serve|schedule/i],
    ["/care", /care|prayer/i], ["/messages", /Your conversations/i], ["/giving", /giving|generosity/i],
    ["/account", /Your space/i], ["/support", /never feel.*stuck/i],
  ];
  for (const [route, text] of routes) {
    await page.goto(route);
    await expect(page.locator("main").getByText(text).first()).toBeVisible();
    await expect(page.locator("select,input[type=date],input[type=time],input[type=datetime-local]")).toHaveCount(0);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, `${route} horizontal overflow`).toBeLessThanOrEqual(1);
    await expectAccessible(page, route);
  }
});

test("branded choice controls replace native menus and support pointer and keyboard use", async ({ page, request, context }, testInfo) => {
  await authenticate(request, context);
  for (const route of ["/participation", "/care", "/messages", "/giving", "/account"]) {
    await page.goto(route);
    await page.waitForLoadState("networkidle");
    await expect(page.locator("select,input[type=date],input[type=time],input[type=datetime-local]")).toHaveCount(0);
  }
  await page.goto("/giving");
  await page.waitForLoadState("networkidle");
  await expect(page.locator(".member-document-skeleton")).toHaveCount(0);
  await expect.poll(() => page.locator("body").evaluate(() => document.readyState)).toBe("complete");
  await page.waitForTimeout(750);
  const year = page.getByText("Year", { exact: true }).locator("..").getByRole("combobox");
  await expect(year).toHaveAttribute("data-ready", "true");
  await year.click();
  await expect(page.getByRole("listbox"), "year pointer menu").toBeVisible();
  if (testInfo.project.name === "mobile-chromium") {
    await expect(page.getByRole("button", { name: "Close choices" }), "mobile close affordance").toBeVisible();
    await page.getByRole("button", { name: "Close choices" }).click();
  } else {
    await year.click();
  }
  await expect(page.getByRole("listbox"), "year pointer menu closes cleanly").toHaveCount(0);
  await year.click();
  await page.keyboard.press("Escape");
  await year.focus();
  await page.keyboard.press("ArrowDown");
  await expect(page.getByRole("listbox"), "year keyboard menu").toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("listbox")).toHaveCount(0);
});

test("offline recovery caches no authenticated member records", async ({ page, request, context }) => {
  await authenticate(request, context);
  await page.goto("/");
  await page.waitForFunction(() => "serviceWorker" in navigator && Boolean(navigator.serviceWorker.controller), null, { timeout: 15_000 }).catch(async () => { await page.reload(); });
  const cached = await page.evaluate(async () => { const keys = await caches.keys(); const urls: string[] = []; for (const key of keys) { const entries = await (await caches.open(key)).keys(); urls.push(...entries.map(item => new URL(item.url).pathname)); } return urls; });
  expect(cached.sort()).toEqual(expect.arrayContaining(["/icon.svg", "/manifest.webmanifest", "/offline"]));
  expect(cached.some(path => path.startsWith("/api/") || ["/account", "/care", "/giving", "/messages"].includes(path))).toBeFalsy();
  await context.setOffline(true);
  await page.goto("/messages");
  await expect(page.getByText("This moment can wait.")).toBeVisible();
  await context.setOffline(false);
});
