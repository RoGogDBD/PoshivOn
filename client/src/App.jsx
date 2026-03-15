import { useEffect, useState } from "react";
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

function CasePreview() {
  return (
    <div className="case-preview">
      <div className="case-preview__window">
        <div className="case-preview__topbar">
          <span className="case-preview__dot" />
          <span className="case-preview__dot" />
          <span className="case-preview__dot" />
        </div>
        <div className="case-preview__layout">
          <div className="case-preview__sidebar">
            <div className="case-preview__pill case-preview__pill--active" />
            <div className="case-preview__pill" />
            <div className="case-preview__pill" />
          </div>
          <div className="case-preview__content">
            <div className="case-preview__line case-preview__line--lg" />
            <div className="case-preview__line" />
            <div className="case-preview__line case-preview__line--short" />
            <div className="case-preview__table">
              <div className="case-preview__row" />
              <div className="case-preview__row" />
              <div className="case-preview__row" />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function App() {
    const [isAuthOpen, setIsAuthOpen] = useState(false);

    useEffect(() => {
        if (isAuthOpen) {
            document.body.classList.add("modal-open");
            return () => {
                document.body.classList.remove("modal-open");
            };
        }

        document.body.classList.remove("modal-open");
        return undefined;
    }, [isAuthOpen]);

    useEffect(() => {
        if (!isAuthOpen) {
            return undefined;
        }

        const buttonContainerId = "yandex-id-button";
        const scriptSrc = "https://yastatic.net/s3/passport-sdk/autofill/v1/sdk-suggest.js";

        const handleKeyDown = (event) => {
            if (event.key === "Escape") {
                setIsAuthOpen(false);
            }
        };

        window.addEventListener("keydown", handleKeyDown);

        const initAuthSuggest = () => {
            if (!window.YaAuthSuggest?.init) {
                return;
            }

            const oauthQueryParams = {
                client_id: 'privet_hacker',
                response_type: 'token',
                redirect_uri: 'https://poshivon.ru/auth'
            };
            const tokenPageOrigin = window.location.origin;

            window.YaAuthSuggest.init(oauthQueryParams, tokenPageOrigin, {
                view: "button",
                parentId: buttonContainerId,
                buttonSize: "xxl",
                buttonView: "main",
                buttonTheme: "light",
                buttonBorderRadius: "28",
                buttonIcon: "ya",
            })
                .then(({ handler }) => handler())
                .then((data) => {
                    console.log("Сообщение с токеном", data);
                })
                .catch((error) => {
                    console.log("Обработка ошибки", error);
                });
        };

        const container = document.getElementById(buttonContainerId);
        if (container) {
            container.innerHTML = "";
        }

        if (window.YaAuthSuggest?.init) {
            initAuthSuggest();
        } else {
            const existingScript = document.querySelector(`script[src="${scriptSrc}"]`);
            if (existingScript) {
                existingScript.addEventListener("load", initAuthSuggest, { once: true });
            } else {
                const script = document.createElement("script");
                script.src = scriptSrc;
                script.async = true;
                script.addEventListener("load", initAuthSuggest, { once: true });
                script.addEventListener(
                    "error",
                    () => {
                        console.log("Не удалось загрузить виджет Яндекс ID.");
                    },
                    { once: true },
                );
                document.body.appendChild(script);
            }
        }

        return () => {
            window.removeEventListener("keydown", handleKeyDown);
        };
    }, [isAuthOpen]);

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
          <button className="btn btn--ghost" type="button" onClick={() => setIsAuthOpen(true)}>
              Вход
          </button>
        </div>
      </header>

      <main>
        <section className="hero">
          <div className="container hero__inner">
            <div className="hero__content">
              <p className="eyebrow">Посчитайте стоимость пошива</p>
              <h1>
                Сервис <span>PoshivOn</span> за <span>2 минуты</span> рассчитает точную себестоимость без ошибок, споров и пересчетов.
              </h1>
              <p className="subtitle">
                Автоматизируйте расчеты в PoshivOn: точная калькуляция ткани,
                фурнитуры и труда — всё в одном сервисе.
              </p>
              <div className="hero__actions">
                <button className="btn btn--primary" type="button">Начать бесплатно</button>
                <a className="btn btn--light" href="#solutions">Смотреть возможности</a>
              </div>
              <div className="hero__stats">
                <div>
                  <span>15 минут</span>
                  <p>на внедрение</p>
                </div>
                <div>
                  <span>-35%</span>
                  <p>ошибок в расчетах</p>
                </div>
                <div>
                  <span>1 окно</span>
                  <p>для технолога и менеджера</p>
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
              <h2>Возможности PoshivOn</h2>
              <p>Настройте расчёт под ваши изделия и процессы.</p>
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
              <CasePreview />
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
              <CasePreview />
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
          </div>
        </div>
      </footer>
        {isAuthOpen && (
            <div
                className="modal-overlay"
                role="presentation"
                onClick={(event) => {
                    if (event.target === event.currentTarget) {
                        setIsAuthOpen(false);
                    }
                }}
            >
                <div className="modal" role="dialog" aria-modal="true" aria-labelledby="auth-title">
                    <button className="modal__close" type="button" onClick={() => setIsAuthOpen(false)} aria-label="Закрыть окно входа">
                        ×
                    </button>
                    <h3 id="auth-title">Вход через Яндекс ID</h3>
                    <div id="yandex-id-button" className="modal__action" />
                </div>
            </div>
        )}
    </div>
  );
}

export default App;
