import { describe, expect, it } from "vitest";

import {
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

  it("uses the shared image pricing copy", () => {
    expect(imagePricingI18nKey("grok", "title")).toBe(
      "admin.groups.imagePricing.title",
    );
  });

  it("keeps Grok image placeholders on the generic image card", () => {
    expect(getImagePricePlaceholder("grok", "image_price_1k")).toBe("0.134");
    expect(getImagePricePlaceholder("grok", "image_price_2k")).toBe("0.201");
  });

  it("keeps non-Grok image placeholders on the generic image card", () => {
    expect(getImagePricePlaceholder("openai", "image_price_1k")).toBe("0.134");
    expect(getDefaultImagePreviewPrice("openai", "image_price_2k")).toBe(0.201);
  });
});
