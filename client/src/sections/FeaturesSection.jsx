const AUDIENCE = [
  {
    role: "ЛПР",
    title: "Руководителям ателье и цехов",
    desc: "Контролируйте загрузку линий и прибыльность каждого заказа, не заглядывая в таблицы.",
    points: [
      "Видите маржу и сроки на старте",
      "Одна формула для всей команды",
      "История расчётов по каждому клиенту",
    ],
  },
  {
    role: "Специалисты",
    title: "Технологам и закройщикам",
    desc: "Настраивайте нормы материалов и операции один раз — дальше расчёт идёт сам.",
    points: [
      "Свои правила по изделиям и операциям",
      "Учёт сложных элементов и фурнитуры",
      "Режимы quick и masterpiece",
    ],
  },
];

const FeaturesSection = () => (
  <section className="section" id="audience">
    <div className="container">
      <div className="section-head">
        <div>
          <div className="section-kicker">Кому это нужно</div>
          <h2 className="section-title">Одна панель — две роли, один результат.</h2>
        </div>
        <p className="section-lede">
          PoshivOn закрывает типовые боли обеих сторон производства: руководителю — прозрачность и
          прибыль, технологу — точные нормы и операции без ручного пересчёта.
        </p>
      </div>
      <div className="aud-grid">
        {AUDIENCE.map((it) => (
          <article className="aud-card" key={it.title}>
            <span className="aud-role">{it.role}</span>
            <h3 className="aud-title">{it.title}</h3>
            <p className="aud-desc">{it.desc}</p>
            <div className="aud-points">
              {it.points.map((p) => (
                <div className="aud-point" key={p}>
                  <span className="aud-point-check">✓</span>
                  {p}
                </div>
              ))}
            </div>
          </article>
        ))}
      </div>
    </div>
  </section>
);

export default FeaturesSection;
