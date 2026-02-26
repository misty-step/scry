import { expect, test } from '@playwright/test';

test.describe('Agent Review Route', () => {
  test('loads without runtime image/surface crashes @smoke', async ({ page }) => {
    const pageErrors: string[] = [];
    page.on('pageerror', (error) => {
      pageErrors.push(error.message);
    });

    await page.goto('/agent');
    await page.waitForLoadState('networkidle');

    // If authenticated and cards are due, kick the session once and ensure no runtime crash.
    const beginSessionButton = page.getByRole('button', { name: /Begin Session/i });
    if (
      (await beginSessionButton.isVisible({ timeout: 1500 }).catch(() => false)) &&
      (await beginSessionButton.isEnabled().catch(() => false))
    ) {
      await beginSessionButton.click();
      await page.waitForTimeout(1500);

      const explainChip = page.getByRole('button', { name: 'Explain this concept' });
      if (await explainChip.isVisible({ timeout: 1500 }).catch(() => false)) {
        await explainChip.click();
        await page.waitForTimeout(1000);
      }
    }

    const combinedErrors = pageErrors.join('\n');
    expect(combinedErrors).not.toMatch(/Invalid src prop/i);
    expect(combinedErrors).not.toMatch(/next-image-unconfigured-host/i);
    expect(combinedErrors).not.toMatch(/Unhandled application error/i);
    expect(combinedErrors).not.toMatch(/Failed to resolve module specifier '@sentry\/nextjs'/i);
  });

  test('does not show / Review navbar breadcrumb @smoke', async ({ page }) => {
    await page.goto('/agent');
    await page.waitForLoadState('domcontentloaded');

    const breadcrumb = page.getByText('/ Review');
    await expect(breadcrumb).toHaveCount(0);
  });

  test('agent chips use deterministic actions and removed legacy labels @smoke', async ({
    page,
  }) => {
    await page.goto('/agent');
    await page.waitForLoadState('networkidle');

    await expect(page.getByText('Discuss this topic')).toHaveCount(0);
    await expect(page.getByText('Scry Agent')).toHaveCount(0);
    await expect(page.getByText('Online')).toHaveCount(0);

    const beginSessionButton = page.getByRole('button', { name: /Begin Session/i });
    const canStart =
      (await beginSessionButton.isVisible({ timeout: 1500 }).catch(() => false)) &&
      (await beginSessionButton.isEnabled().catch(() => false));

    if (!canStart) return;

    await beginSessionButton.click();
    await page.waitForTimeout(1200);

    const rescheduleChip = page.getByRole('button', { name: 'Reschedule' });
    if (!(await rescheduleChip.isVisible({ timeout: 1500 }).catch(() => false))) return;

    await rescheduleChip.click();
    await page.waitForTimeout(1200);

    const chooseInterval = page.getByText('Choose a new interval');
    const plusSevenDays = page.getByRole('button', { name: '+7 days' });
    const scheduleUpdated = page.getByText('Schedule updated');
    const noConceptSelected = page.getByText('No concept selected');
    const hasChooser = await chooseInterval.isVisible({ timeout: 1200 }).catch(() => false);
    const hasPlusSevenDays = await plusSevenDays.isVisible({ timeout: 1200 }).catch(() => false);
    const hasScheduleCard = await scheduleUpdated.isVisible({ timeout: 1200 }).catch(() => false);
    const hasNotice = await noConceptSelected.isVisible({ timeout: 1200 }).catch(() => false);

    expect(hasChooser || hasPlusSevenDays || hasScheduleCard || hasNotice).toBeTruthy();
  });
});
