// Демо-страница /demo — витрина для отдельной услуги (опт футболок).
// Визуальный язык — полноэкранная фото-секция hero с плавающей карточкой и тегами
// (референс: friday-style лендинг), плюс фото-галерея категорий и тёмная секция
// с фичами (референс: zero-space coworking). Функциональность не меняется: опрос-
// виджет DemoQuiz — та же логика, здесь переработана только визуальная обёртка вокруг
// него и остальных секций.
// Черновик для локального согласования — до деплоя не индексируется вместе
// с остальным сайтом (см. index.html: <meta name="robots" content="noindex, nofollow" />).
//
// Фото — свободные (Unsplash License, без атрибуции, без Premium/Unsplash+), прямые
// CDN-ссылки без локального хранения файлов:
const HERO_BG =
  "https://images.unsplash.com/photo-1663433541063-ddab084d1126?auto=format&fit=crop&w=2000&q=80";
const HERO_CARD_IMG =
  "https://images.unsplash.com/photo-1562157873-818bc0726f68?auto=format&fit=crop&w=800&q=80";
const BAND_BG =
  "https://images.unsplash.com/photo-1589793463357-5fb813435467?auto=format&fit=crop&w=2000&q=80";

import DemoQuiz from "../components/DemoQuiz.jsx";

const DECORATION_TAGS = ["Шелкография", "DTF-печать", "Вышивка", "Без нанесения"];

const GALLERY_ITEMS = [
  {
    image: "https://images.unsplash.com/photo-1489987707025-afc232f7ea0f?auto=format&fit=crop&w=800&q=80",
    title: "Футболки",
    caption: "Кулирная гладь, плотность под любой принт",
  },
  {
    image: "https://images.unsplash.com/photo-1620799140188-3b2a02fd9a77?auto=format&fit=crop&w=800&q=80",
    title: "Худи и свитшоты",
    caption: "Футер, тёплая изнанка, плотная посадка",
  },
  {
    image: "https://images.unsplash.com/photo-1588850561407-ed78c282e89b?auto=format&fit=crop&w=800&q=80",
    title: "Кепки и мерч",
    caption: "Вышивка и объёмная печать логотипа",
  },
];

const AUDIENCE_TAGS = [
  { icon: "👕", label: "Бренды одежды" },
  { icon: "🎪", label: "Фестивали и мероприятия" },
  { icon: "☕", label: "Кафе и рестораны" },
  { icon: "🎨", label: "Иллюстраторы и креаторы" },
  { icon: "📹", label: "Блогеры" },
  { icon: "🏆", label: "Спортивные клубы" },
  { icon: "🎓", label: "Учебные заведения" },
  { icon: "💼", label: "Бизнес и промо" },
];

const FEATURES = [
  {
    icon: "🧵",
    title: "Премиальные ткани и бланки",
    description: "Плотный трикотаж, проверенные поставщики полотна",
  },
  {
    icon: "🖨️",
    title: "Полный цикл на месте",
    description: "Макет → печать или вышивка → упаковка, без посредников",
  },
  {
    icon: "🚀",
    title: "От 3 дней на партию",
    description: "Срочные тиражи — без потери качества нанесения",
  },
];

// SVG-фильтр эффекта "жидкого стекла" (реальное преломление фона через
// displacement map, а не просто полупрозрачный фон) — определяется один раз на
// страницу и переиспользуется всеми элементами с классом .demo-glass через
// backdrop-filter: url(#demo-glass-filter). Карта смещения задана в процентах
// (width/height="100%", preserveAspectRatio="none"), поэтому один и тот же
// фильтр растягивается под любой размер элемента — адаптивность не требует
// отдельного фильтра на каждый инстанс. Эффект имеет смысл только поверх
// фото/тёмных зон — на светлом фоне преломлять там нечего, поэтому применяется
// точечно (теги на hero-фото, кнопка и иконка на карточке), а не повсеместно.
const GlassFilterDefs = () => (
  <svg className="demo-glass-filter-defs" aria-hidden="true">
    <defs>
      <filter id="demo-glass-filter" colorInterpolationFilters="sRGB" x="0%" y="0%" width="100%" height="100%">
        <feImage
          x="0"
          y="0"
          width="100%"
          height="100%"
          preserveAspectRatio="none"
          result="map"
          xlinkHref="data:image/png;base64,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"
        />
        <feDisplacementMap in="SourceGraphic" in2="map" result="dispRed" scale="-20" xChannelSelector="R" yChannelSelector="G" />
        <feColorMatrix in="dispRed" type="matrix" values="1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 1 0" result="red" />
        <feDisplacementMap in="SourceGraphic" in2="map" result="dispGreen" scale="-24" xChannelSelector="R" yChannelSelector="G" />
        <feColorMatrix in="dispGreen" type="matrix" values="0 0 0 0 0 0 1 0 0 0 0 0 0 0 0 0 0 0 1 0" result="green" />
        <feDisplacementMap in="SourceGraphic" in2="map" result="dispBlue" scale="-28" xChannelSelector="R" yChannelSelector="G" />
        <feColorMatrix in="dispBlue" type="matrix" values="0 0 0 0 0 0 0 0 0 0 0 0 1 0 0 0 0 0 1 0" result="blue" />
        <feBlend in="red" in2="green" mode="screen" result="rg" />
        <feBlend in="rg" in2="blue" mode="screen" result="output" />
        <feGaussianBlur in="output" stdDeviation="3" />
      </filter>
    </defs>
  </svg>
);

const Demo = () => (
  <div className="page demo-page">
    <GlassFilterDefs />
    <main>
      <section className="demo-hero-photo">
        <div
          className="demo-hero-photo__media"
          style={{ backgroundImage: `url(${HERO_BG})` }}
          aria-hidden="true"
        />
        <div className="demo-hero-photo__scrim" aria-hidden="true" />

        <div className="container demo-hero-photo__nav">
          <a href="/" className="brand demo-hero-photo__brand">
            Poshiv<span className="brand-dot">On</span>
          </a>
          <div className="demo-hero-photo__nav-right">
            <a href="/" className="demo-hero-photo__navlink">
              ← На главную
            </a>
            <a href="#calculator" className="btn btn-primary demo-hero-photo__navcta">
              Оставить заявку
            </a>
          </div>
        </div>

        <div className="container demo-hero-photo__body">
          <div className="demo-hero-photo__copy">
            <div>
              <span className="eyebrow demo-hero-photo__eyebrow">Опт · футболки</span>
              <h1 className="demo-hero-photo__title">
                Пошив футболок
                <br />
                <span>на заказ</span>
                <br />
                <span className="demo-hero-photo__title-muted">от 50 шт. за 3–7 дней</span>
              </h1>
            </div>
            <div>
              <div className="demo-hero-photo__cta">
                <a href="#calculator" className="btn btn-primary btn-lg">
                  Рассчитать стоимость
                </a>
                <a href="#calculator" className="demo-hero-photo__link">
                  Как это работает →
                </a>
              </div>
              <p className="demo-hero-photo__caption">
                Ответьте на пару вопросов о заказе — с вами свяжутся с готовым предложением.
              </p>
            </div>
          </div>

          <div className="demo-hero-photo__card-wrap">
            <div className="demo-hero-photo__card">
              <div
                className="demo-hero-photo__card-img"
                style={{ backgroundImage: `url(${HERO_CARD_IMG})` }}
              />
              <div className="demo-hero-photo__card-bar">
                <span className="demo-hero-photo__card-swatch demo-glass" aria-hidden="true">
                  🧵
                </span>
                <div>
                  <div className="demo-hero-photo__card-title">Тираж 300 шт</div>
                  <div className="demo-hero-photo__card-sub">Шелкография · 2 цвета</div>
                </div>
                <a href="#calculator" className="demo-hero-photo__card-btn demo-glass">
                  Пример →
                </a>
              </div>
            </div>
            <div className="demo-hero-photo__tags">
              {DECORATION_TAGS.map((tag) => (
                <span className="demo-tag demo-tag--on-dark demo-glass" key={tag}>
                  {tag}
                </span>
              ))}
            </div>
          </div>
        </div>
      </section>

      <section className="section demo-intro">
        <div className="container demo-intro-grid">
          <p className="demo-intro-eyebrow">Всё для запуска своей партии</p>
          <p className="demo-intro-statement">
            От первого макета до готовой коробки с футболками — собираем ткань, нанесение,
            тираж и сроки в одну прозрачную стоимость. Работаем и с крупными поставками, и с
            небольшим оптом от 50 шт.
          </p>
        </div>
      </section>

      <section className="section demo-gallery">
        <div className="container">
          <div className="demo-gallery-grid">
            {GALLERY_ITEMS.map((item) => (
              <div className="demo-gallery-item" key={item.title}>
                <div
                  className="demo-gallery-photo"
                  style={{ backgroundImage: `url(${item.image})` }}
                />
                <h3 className="demo-gallery-title">{item.title}</h3>
                <p className="demo-gallery-caption">{item.caption}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="demo-stats">
        <div className="container demo-stats-row">
          <div className="demo-stat-open">
            <span className="demo-stat-open-num">50+</span>
            <span className="demo-stat-open-label">минимальный тираж</span>
          </div>
          <div className="demo-stat-open">
            <span className="demo-stat-open-num">4</span>
            <span className="demo-stat-open-label">способа нанесения</span>
          </div>
          <div className="demo-stat-open">
            <span className="demo-stat-open-num">2 мин</span>
            <span className="demo-stat-open-label">до отправки заявки</span>
          </div>
        </div>
      </section>

      <section className="section demo-audience">
        <div className="container">
          <h2 className="demo-audience-title">Для кого подходит</h2>
          <div className="demo-audience-tags">
            {AUDIENCE_TAGS.map((item) => (
              <span className="demo-tag demo-tag--lg" key={item.label}>
                <span aria-hidden="true">{item.icon}</span> {item.label}
              </span>
            ))}
          </div>
        </div>
      </section>

      <section className="demo-feature-band">
        <div
          className="demo-feature-band__media"
          style={{ backgroundImage: `url(${BAND_BG})` }}
          aria-hidden="true"
        />
        <div className="demo-feature-band__scrim" aria-hidden="true" />
        <div className="container demo-feature-band__inner">
          <div className="demo-feature-band__copy">
            <span className="eyebrow demo-feature-band__eyebrow">Как мы работаем</span>
            <h2 className="demo-feature-band__title">
              Всё нужное для запуска партии — в одном месте
            </h2>
            <p className="demo-feature-band__text">
              Гибкие тиражи, современное оборудование и внимание к деталям — для брендов,
              мероприятий и команд, которым важны сроки и качество.
            </p>
            <a href="#calculator" className="btn btn-primary btn-lg">
              Оставить заявку
            </a>
          </div>
          <div className="demo-feature-band__cards">
            {FEATURES.map((feature) => (
              <div className="demo-feature-card" key={feature.title}>
                <span className="demo-feature-card__icon" aria-hidden="true">
                  {feature.icon}
                </span>
                <div>
                  <div className="demo-feature-card__title">{feature.title}</div>
                  <div className="demo-feature-card__desc">{feature.description}</div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="section demo-calculator" id="calculator">
        <div className="container demo-calculator-inner">
          <div className="demo-calculator-intro">
            <span className="eyebrow">Демо-виджет</span>
            <h2>Попробуйте калькулятор</h2>
            <p className="demo-pricing-text">
              Вопросы появляются по одному, а следующий зависит от вашего ответа — выберите
              «Платье» и отметьте «Карман»: появится отдельный вопрос про вид карманов и
              счётчик количества под каждый выбранный вид, которых не будет для других изделий.
            </p>
          </div>
          <DemoQuiz />
        </div>
      </section>
    </main>
  </div>
);

export default Demo;
