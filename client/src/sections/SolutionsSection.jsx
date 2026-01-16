const SolutionsSection = ({ items }) => (
  <section className="section section--alt solutions-section" id="solutions">
    <div className="container">
      <div className="section__header">
        <h2>Возможности PoshivOn</h2>
        <p>Настройте расчёт под ваши изделия и процессы.</p>
      </div>
      <div className="solution-grid">
        {items.map((item) => (
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
);

export default SolutionsSection;
