import "./App.css";
import solutionCalcIcon from "./assets/solution-calc.svg";
import solutionAutomationIcon from "./assets/solution-automation.svg";
import solutionStockIcon from "./assets/solution-stock.svg";
import solutionComplexIcon from "./assets/solution-complex.svg";

const features = [
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

const solutions = [
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

const cases = [
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

function App() {
  return (
    <div className="page">
      <header className="header">
        <div className="container header__inner">
          <div className="logo">PoshivOn</div>
          <nav className="nav">
            <a href="#about">О платформе</a>
            <a href="#solutions">Решения</a>
            <a href="#cases">Кейсы</a>
            <a href="#cta">Попробовать</a>
          </nav>
          <button className="btn btn--ghost">Вход</button>
        </div>
      </header>

      <main>
        <section className="hero">
          <div className="container hero__inner">
            <div className="hero__content">
              <p className="eyebrow">Посчитайте стоимость пошива</p>
              <h1>
                Сервис <span>PoshivOn</span> за <span>2 минуты</span> расчитает точную себестоимость - без ошибок, споров и пересчетов.
              </h1>
              <p className="subtitle">
                Автоматизируйте расчеты в PoshivOn: точная калькуляция ткани,
                фурнитуры и труда — всё в одном сервисе.
              </p>
              <div className="hero__actions">
                <button className="btn btn--primary">Рассчитать</button>
                <button className="btn btn--light">Демо 3 минуты</button>
              </div>
              <div className="hero__stats">
                <div>
                  <span>15+ моделей</span>
                  <p>Базовые шаблоны расчетов</p>
                </div>
                <div>
                  <span>24/7 поддержка</span>
                  <p>Помогаем с внедрением</p>
                </div>
              </div>
            </div>
            <div className="hero__visual">
              <div className="machine">
                <div className="machine__screen" />
                <div className="machine__details" />
              </div>
              <div className="hero__card">
                <p>Калькуляция</p>
                <strong>₽ 27 450</strong>
                <span>Готово за 4 минуты</span>
              </div>
            </div>
          </div>
        </section>

        <section className="section" id="about">
          <div className="container">
            <h2>Для кого это</h2>
            <div className="feature-grid">
              {features.map((item) => (
                <div className="feature-card" key={item.title}>
                  <h3>{item.title}</h3>
                  <p>{item.description}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="section section--alt solutions-section" id="solutions">
          <div className="container">
            <div className="section__header">
              <h2>Решения от PoshivOn</h2>
              <p>Соберите расчет под свой тип изделий и команду.</p>
            </div>
            <div className="solution-grid">
              {solutions.map((item) => (
                <a className="solution-card" key={item.title} href={item.href} target="_blank" rel="noreferrer">
                  <span className="solution-card__arrow" aria-hidden="true">
                    ↗
                  </span>
                    <div className="solution-card__icon">
                        <img src={item.icon} alt="" aria-hidden="true" />
                    </div>
                    <span className="solution-card__title">{item.title}</span>
                </a>
              ))}
            </div>
          </div>
        </section>

        <section className="section" id="cases">
          <div className="container">
            <h2>Решения от PoshivOn</h2>
            <div className="case-grid">
              <div className="case-preview">скрин</div>
              <div className="case-list">
                {cases.map((item) => (
                  <div className="case-card" key={item.title}>
                    <div className="case-bullet" />
                    <div>
                      <h3>{item.title}</h3>
                      <p>{item.text}</p>
                    </div>
                  </div>
                ))}
              </div>
            </div>
            <div className="case-grid case-grid--reverse">
              <div className="case-preview">скрин</div>
              <div className="case-list">
                {cases.map((item) => (
                  <div className="case-card" key={`${item.title}-second`}>
                    <div className="case-bullet case-bullet--alt" />
                    <div>
                      <h3>{item.title}</h3>
                      <p>{item.text}</p>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </section>

        <section className="cta" id="cta">
          <div className="container cta__inner">
            <h2>Попробуйте бесплатно — посчитайте 3 модели без регистрации.</h2>
            <button className="btn btn--primary">Начать расчет</button>
            <p>
              15 минут, чтобы получить полный расчет с учетом ткани, фурнитуры и
              работы.
            </p>
          </div>
        </section>
      </main>

      <footer className="footer">
        <div className="container footer__inner">
          <div>
            <div className="logo">PoshivOn</div>
            <p>Сервис для расчета стоимости пошива и управления заказами.</p>
          </div>
          <div className="footer__links">
            <a href="#about">О платформе</a>
            <a href="#solutions">Решения</a>
            <a href="#cases">Кейсы</a>
            <a href="#cta">Попробовать</a>
          </div>
          <div className="footer__contacts">
            <span>support@poshivon.ru</span>
            <span>+7 (495) 000-00-00</span>
            <button className="btn btn--ghost">Рассчитать</button>
          </div>
        </div>
      </footer>
    </div>
  );
}

export default App;
