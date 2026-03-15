import solutionCalcIcon from "../assets/solution-calc.svg";
import solutionAutomationIcon from "../assets/solution-automation.svg";
import solutionStockIcon from "../assets/solution-stock.svg";
import solutionComplexIcon from "../assets/solution-complex.svg";

export const navItems = [
  { href: "#about", label: "О платформе" },
  { href: "#solutions", label: "Решения" },
  { href: "#cases", label: "Кейсы" },
  { href: "#cta", label: "Попробовать" },
];

export const features = [
  {
    title: "Руководители цехов",
    description: "Контролируйте загрузку линий и сроки выпуска, не теряя детали.",
  },
  {
    title: "Технологи",
    description: "Собирайте точные нормы материалов и стандартные карты пошива.",
  },
  {
    title: "Заказчики",
    description: "Получайте прозрачный расчет и понятные сроки выполнения заказов.",
  },
];

export const solutions = [
  {
    title: "Объективные расчёты",
    icon: solutionCalcIcon,
    href: "/docs",
  },
  {
    title: "Мгновенный пересчёт",
    icon: solutionAutomationIcon,
    href: "/docs",
  },
  {
    title: "Автоматический учёт всех операций",
    icon: solutionStockIcon,
    href: "/docs",
  },
  {
    title: "Точный учет сложных элементов",
    icon: solutionComplexIcon,
    href: "/docs",
  },
];

export const cases = [
  {
    title: "Как это работает?",
    text: "Заполните параметры заказа, выберите ткань и получите итоговую стоимость без ручного ввода формул.",
  },
  {
    title: "Кому подходит?",
    text: "Мастерским, ателье и фабрикам, которые хотят ускорить подготовку коммерческих предложений.",
  },
  {
    title: "Что в итоге?",
    text: "Четкий расчет, прозрачная прибыль и меньше ошибок при работе с клиентами.",
  },
];

export const footerContacts = [
  {
    label: "support@poshivon.ru",
    href: "mailto:support@poshivon.ru",
  },
  {
    label: "+7 (495) 000-00-00",
    href: "tel:+74950000000",
  },
];
