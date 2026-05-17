// @ts-check
const { defineConfig } = require("@playwright/test");

module.exports = defineConfig({
  testDir: "./tests",
  timeout: 30000,
  use: {
    baseURL: process.env.GOLD_MASTER_ARCHIVE_URL || "http://127.0.0.1:4173",
    headless: true,
  },
  reporter: [["list"], ["html", { open: "never", outputFolder: "../artifacts/playwright-report" }]],
});
