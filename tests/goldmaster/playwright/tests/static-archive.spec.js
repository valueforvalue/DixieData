const { test, expect } = require("@playwright/test");

test("static archive search and detail view keep family links intact", async ({ page }) => {
  await page.goto("/index.html");

  await expect(page.locator("h1")).toContainText("Civil War Research Archive");
  await page.locator("#archive-search").fill("Sarah Carter");
  await expect(page.locator("#result-count")).toContainText("1 record");
  await expect(page.locator(".record-row")).toHaveCount(1);
  await expect(page.locator(".record-row .pill")).toContainText(["Widow"]);

  const familySnapshot = await page.evaluate(() => {
    const records = Array.isArray(window.DIXIE_DATA) ? window.DIXIE_DATA : [];
    const widow = records.find((item) => item.entryType === "widow");
    const linked = records.find((item) => item.displayId === widow?.spouseDisplayId);
    return {
      widowDisplayId: widow?.displayId || "",
      widowDisplayType: widow?.displayType || "",
      linkedDisplayId: linked?.displayId || "",
      linkedName: linked?.name || "",
      archiveHasFamilyLinksTemplate: document.documentElement.innerHTML.includes("Family Links"),
      archiveHasMetadataTemplate: document.documentElement.innerHTML.includes("Archive Metadata"),
    };
  });
  expect(familySnapshot.widowDisplayId).not.toBe("");
  expect(familySnapshot.widowDisplayType).toBe("Widow");
  expect(familySnapshot.linkedDisplayId).not.toBe("");
  expect(familySnapshot.linkedName).toContain("Thomas");
  expect(familySnapshot.archiveHasFamilyLinksTemplate).toBeTruthy();
  expect(familySnapshot.archiveHasMetadataTemplate).toBeTruthy();
});
