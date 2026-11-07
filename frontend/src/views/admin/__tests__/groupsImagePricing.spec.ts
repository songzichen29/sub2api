import { describe, expect, it } from "vitest";

import {
  defaultAllowImageGenerationForPlatform,
  getDefaultImagePreviewPrice,
  getImagePricePlaceholder,
  imagePricingPlatforms,
  imagePricingI18nKey,
  supportsImagePricingPlatform,
} from "../groupsImagePricing";

describe("groups image pricing platform support", () => {
  it("includes Grok image groups", () => {
    expect(supportsImagePricingPlatform("grok")).toBe(true);
    expect(imagePricingPlatforms.has("grok")).toBe(true);
  });

  it("keeps non-media group platforms out of the image pricing controls", () => {
    expect(supportsImagePricingPlatform("anthropic")).toBe(false);
  });

  it("defaults image generation on for Grok and Gemini-family groups only", () => {
    expect(defaultAllowImageGenerationForPlatform("grok")).toBe(true);
    expect(defaultAllowImageGenerationForPlatform("gemini")).toBe(true);
    expect(defaultAllowImageGenerationForPlatform("antigravity")).toBe(true);
    expect(defaultAllowImageGenerationForPlatform("composite")).toBe(false);
    expect(defaultAllowImageGenerationForPlatform("openai")).toBe(false);
  });

  it("includes Composite groups in image pricing controls", () => {
    expect(supportsImagePricingPlatform("composite")).toBe(true);
    expect(imagePricingPlatforms.has("composite")).toBe(true);
  });

  it("uses the shared image pricing copy", () => {
    expect(imagePricingI18nKey("grok", "title")).toBe(
      "admin.groups.imagePricing.title",
    );
  });

  it("uses Grok-specific image placeholders", () => {
    expect(getImagePricePlaceholder("grok", "image_price_1k")).toBe("0.02");
    expect(getImagePricePlaceholder("grok", "image_price_2k")).toBe("0.02");
  });

  it("keeps non-Grok image placeholders on the generic image card", () => {
    expect(getImagePricePlaceholder("openai", "image_price_1k")).toBe("0.134");
    expect(getDefaultImagePreviewPrice("openai", "image_price_2k")).toBe(0.201);
  });
});
