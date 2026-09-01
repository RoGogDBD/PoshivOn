// Дерево вопросов демо-виджета (страница /demo).
//
// Раньше здесь было упрощённое дерево "опт футболок" (продукт → нанесение → тираж →
// срочность) — придуманное для показа самого МЕХАНИЗМА виджета. Сейчас дерево
// перестроено по реальной анкете приёма заказов на индивидуальный пошив (форма на
// forms.yandex.ru, 9 страниц с условной логикой — источник: скриншоты в /home/makar/demo).
// Реальная анкета — не про тираж и цену, а про техническое задание на пошив: тип изделия →
// вид/детали → количество конкретной фурнитуры (карманы, молнии, разрезы) → уточнение
// "другое". По итогам виджет показывает ПРИМЕРНУЮ стоимость (см. calculateEstimate ниже)
// и предлагает оставить контакты для точного расчёта (см. components/DemoQuiz.jsx).
// Цены — иллюстративные, для демонстрации механики "выбор → сумма", не реальный прайс
// PoshivOn; отправка контактов пока никуда не подключена, это черновик.
//
// Три типа шагов:
//   "single" — выбрать один вариант, каждый вариант сам знает свой следующий шаг (next).
//   "multi"  — выбрать несколько вариантов; у части вариантов есть followUp — id шага,
//              который нужно показать, если этот вариант выбран (уточняющий счётчик,
//              текстовое "что ещё", а иногда вложенный multi — как в ветке "Платье", где
//              выбор "Карман" открывает ещё один multi с видами карманов). Все followUp
//              выбранных вариантов показываются по очереди, в порядке списка options,
//              потом виджет переходит на next этого шага.
//   "number" / "text" — простой ввод (количество / свободный текст), после которого либо
//              следующий шаг из очереди followUp-ов (если она не пуста), либо next этого шага.
//
// Ценообразование (calculateEstimate) — тоже декларативное, лежит прямо на шагах/вариантах,
// чтобы не заводить отдельную параллельную таблицу цен:
//   - priceModifier на варианте single/multi без followUp — фиксированная наценка за то,
//     что эта деталь вообще есть (у варианта с followUp своего priceModifier нет — его
//     стоимость целиком считает number-шаг ниже, иначе деталь оплачивалась бы дважды).
//   - unitPrice на number-шаге — цена за штуку, умножается на введённое количество.
//   - flatSurcharge на text-шаге ("Другое") — фиксированная наценка за нестандартную
//     доработку, которую без уточнения не оценить точнее.
//   - priceModifier на варианте корневого шага "garment" — это и есть базовая стоимость
//     пошива изделия без единой доп. детали.
//
// Упрощение сознательно: в реальной форме "Сколько карманов в боковых швах?" — это выбор
// из 1/2 плиткой ("Один вариант"), здесь — как и остальные счётчики — обычное число (мельче
// разница в UI, суть вопроса та же). Ответы, скрытые в форме за "Развернуть ответы" (часть
// списков деталей длиннее показанных 5 пунктов), в дерево не включены — виджет показывает
// то, что было видно на скриншотах.

export const QUIZ_START = "garment";

export const QUIZ_STEPS = {
  // ---- Страница 1: тип изделия ----
  garment: {
    id: "garment",
    type: "single",
    question: "Какое изделие необходимо отшить?",
    subtitle: "Дальше вопросы будут меняться в зависимости от ответа — в этом и есть смысл виджета.",
    options: [
      { label: "Юбка", value: "skirt", next: "skirt-details", priceModifier: 3500 },
      { label: "Брюки", value: "trousers", next: "trousers-type", priceModifier: 4500 },
      { label: "Футболка", value: "tshirt", next: "tshirt-type", priceModifier: 1800 },
      { label: "Рубашка", value: "shirt", next: "shirt-details", priceModifier: 3800 },
      { label: "Пиджак", value: "blazer", next: "blazer-details", priceModifier: 9000 },
      { label: "Пальто / плащ / тренч", value: "coat", next: "outerwear-details", priceModifier: 12000 },
      { label: "Куртка / пуховик", value: "jacket", next: "outerwear-details", priceModifier: 9500 },
      { label: "Платье", value: "dress", next: "dress-length", priceModifier: 6000 },
    ],
  },

  // ---- Юбка ----
  "skirt-details": {
    id: "skirt-details",
    type: "multi",
    question: "Какие детали должны быть на юбке?",
    subtitle: "Можно выбрать сразу несколько.",
    options: [
      { label: "Разрез", value: "cut", followUp: "skirt-cut-count" },
      { label: "Молния", value: "zip", followUp: "skirt-zip-count" },
      { label: "Карман", value: "pocket", followUp: "skirt-pocket-count" },
      { label: "Подклад", value: "lining", priceModifier: 500 },
      { label: "Валан / баска", value: "flounce", priceModifier: 900 },
      { label: "Другое", value: "other", followUp: "skirt-other-text" },
    ],
    next: "result",
  },
  "skirt-cut-count": { id: "skirt-cut-count", type: "number", question: "Сколько должно быть разрезов?", next: "result", unitPrice: 350 },
  "skirt-zip-count": { id: "skirt-zip-count", type: "number", question: "Сколько молний?", next: "result", unitPrice: 400 },
  "skirt-pocket-count": { id: "skirt-pocket-count", type: "number", question: "Сколько карманов?", next: "result", unitPrice: 300 },
  "skirt-other-text": { id: "skirt-other-text", type: "text", question: "Что ещё должно быть на юбке?", next: "result", flatSurcharge: 600 },

  // ---- Брюки ----
  "trousers-type": {
    id: "trousers-type",
    type: "single",
    question: "Какого вида брюки?",
    options: [
      { label: "Джинсы", value: "jeans", next: "jeans-details", priceModifier: 300 },
      { label: "Брюки из костюмной ткани", value: "suit", next: "trousers-details-formal", priceModifier: 900 },
      { label: "Трикотажные брюки", value: "knit", next: "trousers-details-knit", priceModifier: 0 },
      { label: "Другое", value: "other", next: "trousers-details-formal", priceModifier: 500 },
    ],
  },
  "jeans-details": {
    id: "jeans-details",
    type: "multi",
    question: "Какие детали должны быть на джинсах?",
    subtitle: "Можно выбрать сразу несколько.",
    options: [
      { label: "Вышивка", value: "embroidery", priceModifier: 700 },
      { label: "Печворк", value: "patchwork", priceModifier: 900 },
      { label: "Рванина", value: "rips", priceModifier: 500 },
      { label: "Варёнка", value: "stonewash", priceModifier: 400 },
      { label: "Потёртости", value: "distressing", priceModifier: 400 },
      { label: "Другое", value: "other", followUp: "jeans-other-text" },
    ],
    next: "result",
  },
  "jeans-other-text": { id: "jeans-other-text", type: "text", question: "Что ещё должно быть на джинсах?", next: "result", flatSurcharge: 600 },
  "trousers-details-formal": {
    id: "trousers-details-formal",
    type: "multi",
    question: "Какие детали должны быть на брюках?",
    subtitle: "Можно выбрать сразу несколько.",
    options: [
      { label: "Молния", value: "zip", priceModifier: 350 },
      { label: "Карман в шве", value: "side-pocket", followUp: "trousers-side-pocket-count" },
      { label: "Накладной карман", value: "patch-pocket", followUp: "trousers-patch-pocket-count" },
      { label: "Карман в рамку", value: "framed-pocket", followUp: "trousers-framed-pocket-count" },
      { label: "Карман с клапаном", value: "flap-pocket", followUp: "trousers-flap-pocket-count" },
      { label: "Другое", value: "other", followUp: "trousers-other-text" },
    ],
    next: "result",
  },
  "trousers-details-knit": {
    id: "trousers-details-knit",
    type: "multi",
    question: "Какие детали должны быть на брюках?",
    subtitle: "Можно выбрать сразу несколько.",
    options: [
      { label: "Шнурок", value: "drawstring", priceModifier: 200 },
      { label: "Манжеты", value: "cuffs", priceModifier: 350 },
      { label: "Гульфик", value: "fly", priceModifier: 300 },
      { label: "Накладной карман", value: "patch-pocket", followUp: "trousers-patch-pocket-count" },
      { label: "Карман в шве", value: "side-pocket", followUp: "trousers-side-pocket-count" },
      { label: "Другое", value: "other", followUp: "trousers-other-text" },
    ],
    next: "result",
  },
  "trousers-side-pocket-count": { id: "trousers-side-pocket-count", type: "number", question: "Сколько должно быть карманов в боковых швах?", next: "result", unitPrice: 250 },
  "trousers-patch-pocket-count": { id: "trousers-patch-pocket-count", type: "number", question: "Сколько должно быть накладных карманов?", next: "result", unitPrice: 300 },
  "trousers-framed-pocket-count": { id: "trousers-framed-pocket-count", type: "number", question: "Сколько должно быть карманов в рамку?", next: "result", unitPrice: 450 },
  "trousers-flap-pocket-count": { id: "trousers-flap-pocket-count", type: "number", question: "Сколько карманов должны иметь клапан?", next: "result", unitPrice: 400 },
  "trousers-other-text": { id: "trousers-other-text", type: "text", question: "Что ещё должно быть на брюках?", next: "result", flatSurcharge: 600 },

  // ---- Футболка ----
  "tshirt-type": {
    id: "tshirt-type",
    type: "single",
    question: "Какой вид футболки?",
    options: [
      { label: "Простая трикотажная", value: "basic", next: "result", priceModifier: 0 },
      { label: "Поло", value: "polo", next: "tshirt-collar", priceModifier: 400 },
      { label: "Текстильная", value: "woven", next: "result", priceModifier: 300 },
      { label: "Другое", value: "other", next: "tshirt-other-text", priceModifier: 500 },
    ],
  },
  "tshirt-other-text": { id: "tshirt-other-text", type: "text", question: "Что должно быть на футболке?", next: "result", flatSurcharge: 500 },
  "tshirt-collar": {
    id: "tshirt-collar",
    type: "multi",
    question: "Что должно быть на воротнике?",
    subtitle: "Можно выбрать сразу несколько.",
    options: [
      { label: "Молния", value: "zip", priceModifier: 250 },
      { label: "Пуговицы", value: "buttons", priceModifier: 200 },
      { label: "Вышивка", value: "embroidery", priceModifier: 600 },
      { label: "Ничего", value: "none", priceModifier: 0 },
      { label: "Другое", value: "other", followUp: "tshirt-collar-other-text" },
    ],
    next: "result",
  },
  "tshirt-collar-other-text": { id: "tshirt-collar-other-text", type: "text", question: "Что должно быть на футболке?", next: "result", flatSurcharge: 500 },

  // ---- Рубашка ----
  "shirt-details": {
    id: "shirt-details",
    type: "multi",
    question: "Какие детали должны быть на рубашке?",
    subtitle: "Можно выбрать сразу несколько.",
    options: [
      { label: "Накладной карман", value: "patch-pocket", followUp: "shirt-patch-pocket-count" },
      { label: "Карман с клапаном", value: "flap-pocket", followUp: "shirt-flap-pocket-count" },
      { label: "Складка / сборка", value: "pleat", priceModifier: 400 },
      { label: "Другое", value: "other", followUp: "shirt-other-text" },
    ],
    next: "result",
  },
  "shirt-patch-pocket-count": { id: "shirt-patch-pocket-count", type: "number", question: "Сколько должно быть накладных карманов?", next: "result", unitPrice: 300 },
  "shirt-flap-pocket-count": { id: "shirt-flap-pocket-count", type: "number", question: "Сколько карманов должны иметь клапан?", next: "result", unitPrice: 400 },
  "shirt-other-text": { id: "shirt-other-text", type: "text", question: "Что ещё должно быть на рубашке?", next: "result", flatSurcharge: 600 },

  // ---- Пиджак / жакет ----
  "blazer-details": {
    id: "blazer-details",
    type: "multi",
    question: "Какие детали должны быть на пиджаке/жакете?",
    subtitle: "Можно выбрать сразу несколько.",
    options: [
      { label: "2 борта", value: "double-breasted", priceModifier: 900 },
      { label: "Подклад", value: "lining", priceModifier: 700 },
      { label: "Накладной карман", value: "patch-pocket", followUp: "blazer-patch-pocket-count" },
      { label: "Карман в рамку", value: "framed-pocket", followUp: "blazer-framed-pocket-count" },
      { label: "Карман с клапаном", value: "flap-pocket", followUp: "blazer-flap-pocket-count" },
      { label: "Другое", value: "other", followUp: "blazer-other-text" },
    ],
    next: "result",
  },
  "blazer-patch-pocket-count": { id: "blazer-patch-pocket-count", type: "number", question: "Сколько накладных карманов?", next: "result", unitPrice: 350 },
  "blazer-framed-pocket-count": { id: "blazer-framed-pocket-count", type: "number", question: "Сколько карманов в рамку?", next: "result", unitPrice: 500 },
  "blazer-flap-pocket-count": { id: "blazer-flap-pocket-count", type: "number", question: "Сколько карманов должно быть с клапаном?", next: "result", unitPrice: 450 },
  "blazer-other-text": { id: "blazer-other-text", type: "text", question: "Что ещё должно быть на пиджаке/жакете?", next: "result", flatSurcharge: 700 },

  // ---- Пальто/плащ/тренч и куртка/пуховик — общая ветка (как в реальной форме) ----
  "outerwear-details": {
    id: "outerwear-details",
    type: "multi",
    question: "Какие детали должны быть на изделии?",
    subtitle: "Можно выбрать сразу несколько.",
    options: [
      { label: "Капюшон", value: "hood", priceModifier: 900 },
      { label: "2 борта", value: "double-breasted", priceModifier: 900 },
      { label: "Накладной карман", value: "patch-pocket", followUp: "outerwear-patch-pocket-count" },
      { label: "Карман в рамку", value: "framed-pocket", followUp: "outerwear-framed-pocket-count" },
      { label: "Карман с клапаном", value: "flap-pocket", followUp: "outerwear-flap-pocket-count" },
      { label: "Карман в боковом шве", value: "side-pocket", followUp: "outerwear-side-pocket-count" },
      { label: "Другое", value: "other", followUp: "outerwear-other-text" },
    ],
    next: "result",
  },
  "outerwear-patch-pocket-count": { id: "outerwear-patch-pocket-count", type: "number", question: "Сколько должно быть накладных карманов?", next: "result", unitPrice: 350 },
  "outerwear-framed-pocket-count": { id: "outerwear-framed-pocket-count", type: "number", question: "Сколько должно быть карманов в рамку?", next: "result", unitPrice: 500 },
  "outerwear-flap-pocket-count": { id: "outerwear-flap-pocket-count", type: "number", question: "Сколько карманов должно быть с клапаном?", next: "result", unitPrice: 450 },
  "outerwear-side-pocket-count": { id: "outerwear-side-pocket-count", type: "number", question: "Сколько карманов должно быть в боковых швах?", next: "result", unitPrice: 300 },
  "outerwear-other-text": { id: "outerwear-other-text", type: "text", question: "Что ещё должно быть на изделии?", next: "result", flatSurcharge: 800 },

  // ---- Платье ----
  "dress-length": {
    id: "dress-length",
    type: "single",
    question: "Какой длины платье?",
    options: [
      { label: "Мини", value: "mini", next: "dress-details", priceModifier: 0 },
      { label: "Миди (по колено)", value: "midi", next: "dress-details", priceModifier: 300 },
      { label: "Макси", value: "maxi", next: "dress-details", priceModifier: 600 },
    ],
  },
  "dress-details": {
    id: "dress-details",
    type: "multi",
    question: "Какие детали должны быть на платье?",
    subtitle: "Можно выбрать сразу несколько.",
    options: [
      { label: "Воротник", value: "collar", priceModifier: 300 },
      { label: "Рукава", value: "sleeves", priceModifier: 400 },
      { label: "Молния", value: "zip", priceModifier: 350 },
      { label: "Пуговицы", value: "buttons", priceModifier: 250 },
      { label: "Манжеты", value: "cuffs", priceModifier: 300 },
      { label: "Карман", value: "pocket", followUp: "dress-pocket-type" },
      { label: "Другое", value: "other", followUp: "dress-other-text" },
    ],
    next: "result",
  },
  // Вложенный multi: реальная форма сначала спрашивает категорию "Карман" в общем списке
  // деталей, и только если она выбрана — уточняет вид карманов отдельным шагом. Виджет
  // умеет так же: followUp может сам быть multi-шагом со своими followUp-ами.
  "dress-pocket-type": {
    id: "dress-pocket-type",
    type: "multi",
    question: "Какие должны быть карманы?",
    subtitle: "Можно выбрать сразу несколько.",
    options: [
      { label: "Накладные", value: "patch", followUp: "dress-patch-pocket-count" },
      { label: "В рамку", value: "framed", followUp: "dress-framed-pocket-count" },
      { label: "С клапаном", value: "flap", followUp: "dress-flap-pocket-count" },
      { label: "В боковых швах", value: "side", followUp: "dress-side-pocket-count" },
    ],
    next: "result",
  },
  "dress-patch-pocket-count": { id: "dress-patch-pocket-count", type: "number", question: "Сколько должно быть накладных карманов?", next: "result", unitPrice: 300 },
  "dress-framed-pocket-count": { id: "dress-framed-pocket-count", type: "number", question: "Сколько должно быть карманов в рамку?", next: "result", unitPrice: 450 },
  "dress-flap-pocket-count": { id: "dress-flap-pocket-count", type: "number", question: "Сколько карманов должно быть с клапаном?", next: "result", unitPrice: 400 },
  "dress-side-pocket-count": { id: "dress-side-pocket-count", type: "number", question: "Сколько должно быть карманов в боковых швах?", next: "result", unitPrice: 250 },
  "dress-other-text": { id: "dress-other-text", type: "text", question: "Что ещё должно быть у платья?", next: "result", flatSurcharge: 600 },
};

// Сумма ответов, а не отдельная параллельная модель: идём по всем данным ответам, для
// каждого смотрим тип его шага и складываем ту наценку, что там определена (см. комментарий
// про priceModifier/unitPrice/flatSurcharge вверху файла). Округляем до сотни — это оценка
// "на глаз", а не точный расчёт, для точного нужны контакты (см. DemoQuiz.jsx).
export function calculateEstimate(answers) {
  let total = 0;
  for (const [stepId, answer] of Object.entries(answers)) {
    const step = QUIZ_STEPS[stepId];
    if (!step || answer == null) continue;
    if (step.type === "single") {
      total += answer.priceModifier ?? 0;
    } else if (step.type === "multi") {
      for (const option of answer) total += option.priceModifier ?? 0;
    } else if (step.type === "number") {
      total += (Number(answer) || 0) * (step.unitPrice ?? 0);
    } else if (step.type === "text") {
      total += step.flatSurcharge ?? 0;
    }
  }
  return Math.max(0, Math.round(total / 100) * 100);
}
