import { useState, useEffect } from "react";

const HERO_TAGS = [
  "Калькулятор пошива",
  "Себестоимость · AI",
  "Раскладка · ткань",
  "Операции · тариф",
  "Сегмент · маржа",
  "Партии · сроки",
];

function EyebrowRotator() {
  const [i, setI] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setI((v) => (v + 1) % HERO_TAGS.length), 2400);
    return () => clearInterval(id);
  }, []);
  return (
    <div className="eyebrow eyebrow--rot" aria-live="polite">
      <span className="eyebrow-stage">
        <span className="eyebrow-sizer" aria-hidden="true">
          {HERO_TAGS.reduce((a, b) => (a.length > b.length ? a : b))}
        </span>
        {HERO_TAGS.map((t, idx) => (
          <span
            key={idx}
            className={
              "eyebrow-word " +
              (idx === i
                ? "is-in"
                : idx === (i - 1 + HERO_TAGS.length) % HERO_TAGS.length
                ? "is-out"
                : "")
            }
          >
            {t}
          </span>
        ))}
      </span>
    </div>
  );
}

function HeroMock() {
  return (
    <div className="mock--machine" role="img" aria-label="Цифровая швейная машина — визуал PoshivOn">
      <div className="sm-wrap">
        <div className="sm-glow" aria-hidden="true" />
        <img className="sm-img" src="/sewing-machine.png" alt="" />
        <div className="sm-spark sp1" aria-hidden="true" />
        <div className="sm-spark sp2" aria-hidden="true" />
        <div className="sm-spark sp3" aria-hidden="true" />
        <div className="sm-chip sm-chip-1" aria-hidden="true">
          <span className="sm-chip-k">Расчёт</span>
          <span className="sm-chip-v">1.8 с</span>
        </div>
        <div className="sm-chip sm-chip-2" aria-hidden="true">
          <span className="sm-chip-k">Операций</span>
          <span className="sm-chip-v">12</span>
        </div>
        <div className="sm-chip sm-chip-3" aria-hidden="true">
          <span className="sm-chip-k">Маржа</span>
          <span className="sm-chip-v sm-ok">+35%</span>
        </div>
      </div>
    </div>
  );
}

const HeroSection = ({ onAuthOpen }) => (
  <section className="hero">
    <div className="container hero-grid">
      <div>
        <EyebrowRotator />
        <h1 className="hero-title">
          Точная себестоимость пошива — <span>за две минуты.</span>
        </h1>
        <p className="hero-sub">
          PoshivOn собирает ткань, фурнитуру, операции и срочность в одной панели.
          Вы видите итог до начала пошива, а не после.
        </p>
        <div className="hero-cta">
          <button className="btn btn-primary btn-lg" type="button" onClick={onAuthOpen}>
            Рассчитать заказ →
          </button>
        </div>
        <div className="hero-meta">
          <div>
            <div className="hero-meta-num">
              2 <span className="u">мин</span>
            </div>
            <div className="hero-meta-label">от заказа до итога</div>
          </div>
          <div>
            <div className="hero-meta-num">
              −40<span className="u">%</span>
            </div>
            <div className="hero-meta-label">ошибок в расчётах</div>
          </div>
          <div>
            <div className="hero-meta-num">AI</div>
            <div className="hero-meta-label">оценка сегмента</div>
          </div>
        </div>
      </div>
      <div>
        <HeroMock />
      </div>
    </div>
  </section>
);

export default HeroSection;
