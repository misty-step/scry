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
  });

  test('does not show / Review navbar breadcrumb @smoke', async ({ page }) => {
    await page.goto('/agent');
    await page.waitForLoadState('domcontentloaded');

    const breadcrumb = page.getByText('/ Review');
    await expect(breadcrumb).toHaveCount(0);
  });
});
