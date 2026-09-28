import { describe, expect, it } from "vitest";
import { QUIZ_STEPS, calculateEstimate, pickBatchDiscount } from "./demoQuiz.js";

const skirt = QUIZ_STEPS.garment.options.find((option) => option.value === "skirt");
const lining = QUIZ_STEPS["skirt-details"].options.find((option) => option.value === "lining");

describe("pickBatchDiscount", () => {
  it("follows the default production tiers from costing.go", () => {
    expect(pickBatchDiscount(1)).toBe(0);
    expect(pickBatchDiscount(10)).toBe(0);
    expect(pickBatchDiscount(11)).toBe(5);
    expect(pickBatchDiscount(50)).toBe(5);
    expect(pickBatchDiscount(51)).toBe(10);
    expect(pickBatchDiscount(101)).toBe(12);
  });
});

describe("calculateEstimate", () => {
  it("multiplies the per-item price by the batch size and applies the batch discount", () => {
    // Юбка 3500 + подклад 500 = 4000 за штуку; 20 шт. → 80 000, скидка 5% → 76 000.
    const estimate = calculateEstimate({ garment: skirt, "skirt-details": [lining], "batch-size": "20" });
    expect(estimate).toEqual({
      quantity: 20,
      discountPercent: 5,
      pricePerUnit: 3800,
      subtotal: 80000,
      discountAmount: 4000,
      total: 76000,
    });
  });

  it("does not add the batch size to the price as if it were a detail counter", () => {
    const estimate = calculateEstimate({ garment: skirt, "batch-size": "3" });
    expect(estimate.total).toBe(3500 * 3);
    expect(estimate.pricePerUnit).toBe(3500);
  });

  it("treats a missing or invalid batch size as a single item", () => {
    expect(calculateEstimate({ garment: skirt }).quantity).toBe(1);
    expect(calculateEstimate({ garment: skirt, "batch-size": "0" }).quantity).toBe(1);
  });

  it("routes every branch through the batch-size step before the result", () => {
    const leaksToResult = Object.values(QUIZ_STEPS).filter(
      (step) => step.id !== "batch-size" && (step.next === "result" || step.options?.some((option) => option.next === "result")),
    );
    expect(leaksToResult).toEqual([]);
  });
});
