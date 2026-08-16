import { describe, expect, it } from "vitest";

import {
  DEFAULT_GARMENT_NAMES,
  DEFAULT_OPERATION_NAMES,
  defaultSettings,
  getDefaultDiscountRange,
  isBlankName,
  isDuplicateName,
  validateDiscountFields,
  validateGarmentFields,
  validateOperationFields,
} from "./Panel.jsx";

// Значения из server/internal/service/costing.go:227-242 (DefaultUserSettings).
// Дублируются здесь намеренно: тест должен падать, если клиентские списки разъедутся
// с сервером — а не «подстраиваться» под то, что написано в Panel.jsx.
const SERVER_GARMENT_NAMES = ["Пиджак", "Юбка", "Рубашка", "Платье"];
const SERVER_OPERATION_NAMES = [
  "Карман накладной",
  "Карман прорезной",
  "Подклад",
  "Потайная молния",
  "Воротник",
  "Манжеты",
  "Шлица",
  "Декоративная отстрочка",
];

const validGarment = { base_minutes: 120, complexity_coeff: 1.2, quick_price: 4200 };
const validOperation = { additional_minutes: 15, additional_material_per_unit: 80, quick_percent: 8 };
const validDiscount = { min_qty: 11, max_qty: 50, percent: 5 };

describe("isDuplicateName", () => {
  it("detects exact, case-insensitive, and whitespace-padded duplicate names", () => {
    expect(isDuplicateName("Пиджак", defaultSettings.garments)).toBe(true);
    expect(isDuplicateName("пиджак", defaultSettings.garments)).toBe(true);
    expect(isDuplicateName("ПИДЖАК", defaultSettings.garments)).toBe(true);
    expect(isDuplicateName("  Пиджак  ", defaultSettings.garments)).toBe(true);
    expect(isDuplicateName(" пиджак ", defaultSettings.garments)).toBe(true);
  });

  it("matches case-insensitively against a key that itself has padding", () => {
    expect(isDuplicateName("Жилет", { "  Жилет  ": {} })).toBe(true);
  });

  it("allows a genuinely distinct name", () => {
    expect(isDuplicateName("Пальто", defaultSettings.garments)).toBe(false);
    expect(isDuplicateName("Пиджак-жилет", defaultSettings.garments)).toBe(false);
    expect(isDuplicateName("Шлица", defaultSettings.garments)).toBe(false);
  });

  it("treats an empty collection as having no duplicates", () => {
    expect(isDuplicateName("Пиджак", {})).toBe(false);
    expect(isDuplicateName("Пиджак", undefined)).toBe(false);
  });
});

describe("isBlankName", () => {
  it("flags empty and whitespace-only names, matching the backend's TrimSpace check", () => {
    expect(isBlankName("")).toBe(true);
    expect(isBlankName("   ")).toBe(true);
    expect(isBlankName("\t\n")).toBe(true);
    expect(isBlankName(undefined)).toBe(true);
  });

  it("accepts a non-empty name", () => {
    expect(isBlankName("Пальто")).toBe(false);
    expect(isBlankName("  Пальто  ")).toBe(false);
  });
});

describe("validateGarmentFields", () => {
  it("accepts a fully valid garment", () => {
    expect(validateGarmentFields(validGarment)).toEqual({ valid: true, errors: {} });
  });

  it("rejects quick_price <= 0 (stricter than the backend's save-time >= 0)", () => {
    for (const quick_price of [0, -0.01, -1]) {
      const result = validateGarmentFields({ ...validGarment, quick_price });
      expect(result.valid).toBe(false);
      expect(result.errors.quick_price).toBeTruthy();
    }
  });

  it("rejects base_minutes <= 0", () => {
    for (const base_minutes of [0, -0.01, -30]) {
      const result = validateGarmentFields({ ...validGarment, base_minutes });
      expect(result.valid).toBe(false);
      expect(result.errors.base_minutes).toBeTruthy();
    }
  });

  it("rejects complexity_coeff <= 0", () => {
    for (const complexity_coeff of [0, -0.01, -1.6]) {
      const result = validateGarmentFields({ ...validGarment, complexity_coeff });
      expect(result.valid).toBe(false);
      expect(result.errors.complexity_coeff).toBeTruthy();
    }
  });

  it("accepts the smallest positive values", () => {
    // base_minutes и quick_price — int/int64 на сервере, поэтому «наименьшее допустимое»
    // для них 1, а не 0.01; дробным остаётся только complexity_coeff (float64).
    expect(validateGarmentFields({ base_minutes: 1, complexity_coeff: 0.01, quick_price: 1 }).valid).toBe(true);
  });

  it("rejects fractional base_minutes and quick_price (int/int64 on the server)", () => {
    for (const base_minutes of [10.5, "10.5", 0.5, 259.999]) {
      const result = validateGarmentFields({ ...validGarment, base_minutes });
      expect(result.valid).toBe(false);
      expect(result.errors.base_minutes).toBeTruthy();
    }

    for (const quick_price of [7000.5, "7000.5", 0.01]) {
      const result = validateGarmentFields({ ...validGarment, quick_price });
      expect(result.valid).toBe(false);
      expect(result.errors.quick_price).toBeTruthy();
    }
  });

  it("still allows a fractional complexity_coeff (float64 on the server)", () => {
    expect(validateGarmentFields({ ...validGarment, complexity_coeff: 1.15 }).valid).toBe(true);
    expect(validateGarmentFields({ ...validGarment, complexity_coeff: "1.6" }).valid).toBe(true);
  });

  it("rejects empty, blank and non-numeric input instead of coercing it to 0", () => {
    for (const bad of ["", "   ", "abc", null, undefined, NaN, Infinity]) {
      const result = validateGarmentFields({ ...validGarment, quick_price: bad });
      expect(result.valid).toBe(false);
      expect(result.errors.quick_price).toBeTruthy();
    }
  });

  it("accepts numeric strings, since form inputs deliver strings", () => {
    expect(validateGarmentFields({ base_minutes: "120", complexity_coeff: "1.2", quick_price: "4200" }).valid).toBe(true);
  });

  it("reports every failing field at once", () => {
    const result = validateGarmentFields({ base_minutes: 0, complexity_coeff: -1, quick_price: 0 });
    expect(result.valid).toBe(false);
    expect(Object.keys(result.errors).sort()).toEqual(["base_minutes", "complexity_coeff", "quick_price"]);
  });
});

describe("validateOperationFields", () => {
  it("accepts a fully valid operation", () => {
    expect(validateOperationFields(validOperation)).toEqual({ valid: true, errors: {} });
  });

  it("allows 0 for every field", () => {
    const result = validateOperationFields({
      additional_minutes: 0,
      additional_material_per_unit: 0,
      quick_percent: 0,
    });
    expect(result).toEqual({ valid: true, errors: {} });
  });

  it("rejects any field below 0", () => {
    for (const field of ["additional_minutes", "additional_material_per_unit", "quick_percent"]) {
      for (const bad of [-0.01, -1]) {
        const result = validateOperationFields({ ...validOperation, [field]: bad });
        expect(result.valid).toBe(false);
        expect(result.errors[field]).toBeTruthy();
      }
    }
  });

  it("rejects empty and non-numeric input instead of coercing it to 0", () => {
    for (const bad of ["", "   ", "abc", null, undefined, NaN]) {
      const result = validateOperationFields({ ...validOperation, quick_percent: bad });
      expect(result.valid).toBe(false);
      expect(result.errors.quick_percent).toBeTruthy();
    }
  });

  it("rejects fractional additional_minutes and additional_material_per_unit (int/int64 on the server)", () => {
    for (const field of ["additional_minutes", "additional_material_per_unit"]) {
      for (const bad of [15.5, "15.5", 0.5]) {
        const result = validateOperationFields({ ...validOperation, [field]: bad });
        expect(result.valid).toBe(false);
        expect(result.errors[field]).toBeTruthy();
      }
      // 0 остаётся валидным: это целое, а операция «только проценты» — штатный случай.
      expect(validateOperationFields({ ...validOperation, [field]: 0 }).valid).toBe(true);
    }
  });

  it("still allows a fractional quick_percent (float64 on the server)", () => {
    expect(validateOperationFields({ ...validOperation, quick_percent: 7.5 }).valid).toBe(true);
    expect(validateOperationFields({ ...validOperation, quick_percent: "8.25" }).valid).toBe(true);
  });
});

describe("validateDiscountFields", () => {
  it("accepts a fully valid tier", () => {
    expect(validateDiscountFields(validDiscount)).toEqual({ valid: true, errors: {} });
  });

  it("rejects min_qty <= 0", () => {
    for (const min_qty of [0, -1]) {
      const result = validateDiscountFields({ ...validDiscount, min_qty });
      expect(result.valid).toBe(false);
      expect(result.errors.min_qty).toBeTruthy();
    }
  });

  it("rejects max_qty < min_qty but allows max_qty === min_qty", () => {
    const tooSmall = validateDiscountFields({ min_qty: 11, max_qty: 10, percent: 5 });
    expect(tooSmall.valid).toBe(false);
    expect(tooSmall.errors.max_qty).toBeTruthy();

    expect(validateDiscountFields({ min_qty: 11, max_qty: 11, percent: 5 }).valid).toBe(true);
  });

  it("rejects percent outside [0, 100] and allows both bounds", () => {
    for (const percent of [-0.01, -1, 100.01, 101]) {
      const result = validateDiscountFields({ ...validDiscount, percent });
      expect(result.valid).toBe(false);
      expect(result.errors.percent).toBeTruthy();
    }

    expect(validateDiscountFields({ ...validDiscount, percent: 0 }).valid).toBe(true);
    expect(validateDiscountFields({ ...validDiscount, percent: 100 }).valid).toBe(true);
  });

  it("rejects empty and non-numeric input instead of coercing it to 0", () => {
    for (const bad of ["", "   ", "abc", null, undefined, NaN]) {
      expect(validateDiscountFields({ ...validDiscount, min_qty: bad }).valid).toBe(false);
      expect(validateDiscountFields({ ...validDiscount, percent: bad }).valid).toBe(false);
    }
  });

  // Findings U1: у max_qty своя ветка проверки на положительность и «число вообще», и до сих пор
  // она не была покрыта напрямую — покрытие держалось на сравнении с min_qty. Здесь оба пути
  // проверяются явно и по отдельности, чтобы ветка не осталась незамеченной, если сравнение
  // когда-нибудь перепишут.
  it("rejects empty and non-numeric max_qty on its own branch", () => {
    for (const bad of ["", "   ", "abc", null, undefined, NaN, Infinity]) {
      const result = validateDiscountFields({ min_qty: 11, max_qty: bad, percent: 5 });
      expect(result.valid).toBe(false);
      expect(result.errors.max_qty).toBeTruthy();
    }
  });

  it("rejects max_qty <= 0 even when min_qty is valid", () => {
    for (const max_qty of [0, -1, "-5"]) {
      const result = validateDiscountFields({ min_qty: 11, max_qty, percent: 5 });
      expect(result.valid).toBe(false);
      expect(result.errors.max_qty).toBe("«До» должно быть больше 0");
    }
  });

  it("rejects fractional min_qty and max_qty (int on the server)", () => {
    const badMin = validateDiscountFields({ min_qty: 10.5, max_qty: 50, percent: 5 });
    expect(badMin.valid).toBe(false);
    expect(badMin.errors.min_qty).toBe("«От» должно быть целым числом");

    // Дробное max_qty больше min_qty: сравнение диапазона его пропускает, отклоняет именно
    // проверка на целое — та самая новая ветка.
    for (const max_qty of [50.5, "50.5", 11.000001]) {
      const result = validateDiscountFields({ min_qty: 11, max_qty, percent: 5 });
      expect(result.valid).toBe(false);
      expect(result.errors.max_qty).toBe("«До» должно быть целым числом");
    }
  });

  it("reports the range error, not the integer error, for a whole max_qty below min_qty", () => {
    const result = validateDiscountFields({ min_qty: 11, max_qty: 10, percent: 5 });
    expect(result.valid).toBe(false);
    expect(result.errors.max_qty).toBe("«До» не может быть меньше «От»");
  });

  it("still allows a fractional percent (float64 on the server)", () => {
    expect(validateDiscountFields({ ...validDiscount, percent: 7.5 }).valid).toBe(true);
    expect(validateDiscountFields({ ...validDiscount, percent: "12.25" }).valid).toBe(true);
  });
});

describe("default name constants", () => {
  it("DEFAULT_GARMENT_NAMES has exactly the 4 server-matching names", () => {
    expect(DEFAULT_GARMENT_NAMES).toEqual(SERVER_GARMENT_NAMES);
  });

  it("DEFAULT_OPERATION_NAMES has exactly the 8 server-matching names", () => {
    expect(DEFAULT_OPERATION_NAMES).toEqual(SERVER_OPERATION_NAMES);
  });

  it("keeps defaultSettings.garments in sync with the server defaults", () => {
    expect(Object.keys(defaultSettings.garments).sort()).toEqual([...SERVER_GARMENT_NAMES].sort());
  });

  it("keeps defaultSettings.operations in sync with the server defaults", () => {
    // Регрессия на уже случившийся дрейф: в defaultSettings.operations не хватало
    // «Шлица» и «Декоративная отстрочка» (6 из 8).
    expect(Object.keys(defaultSettings.operations).sort()).toEqual([...SERVER_OPERATION_NAMES].sort());
  });
});

describe("getDefaultDiscountRange", () => {
  it("computes both min_qty and max_qty from the last tier", () => {
    expect(
      getDefaultDiscountRange([
        { min_qty: 1, max_qty: 10, percent: 0 },
        { min_qty: 11, max_qty: 50, percent: 5 },
        { min_qty: 51, max_qty: 100, percent: 10 },
      ])
    ).toEqual({ min_qty: 101, max_qty: 101 });
  });

  it("uses the last tier in array order, not the largest max_qty", () => {
    expect(
      getDefaultDiscountRange([
        { min_qty: 51, max_qty: 100, percent: 10 },
        { min_qty: 1, max_qty: 10, percent: 0 },
      ])
    ).toEqual({ min_qty: 11, max_qty: 11 });
  });

  it("returns min_qty=1, max_qty=1 for an empty tier list", () => {
    expect(getDefaultDiscountRange([])).toEqual({ min_qty: 1, max_qty: 1 });
  });

  it("falls back to 1 instead of throwing on a missing or malformed tier list", () => {
    expect(getDefaultDiscountRange(undefined)).toEqual({ min_qty: 1, max_qty: 1 });
    expect(getDefaultDiscountRange([{ min_qty: 1, percent: 0 }])).toEqual({ min_qty: 1, max_qty: 1 });
  });

  it("never suggests a range the discount validator would reject", () => {
    const suggestion = getDefaultDiscountRange(defaultSettings.batch_discounts);
    expect(validateDiscountFields({ ...suggestion, percent: 0 })).toEqual({ valid: true, errors: {} });
  });
});
