const CAPS = [
  { ix: "01", t: "Объективные расчёты", d: "Нормы, операции и коэффициенты в одной модели — без скрытых формул." },
  { ix: "02", t: "Мгновенный пересчёт", d: "Поменяли ткань, тираж или срочность — итог обновляется за секунду." },
  { ix: "03", t: "Учёт всех операций", d: "От подклада до лацкана. Сложные элементы считаются отдельно." },
  { ix: "04", t: "AI-оценка рынка", d: "DeepSeek подсказывает сегмент, диапазон цены и риски позиционирования." },
];

const SolutionsSection = () => (
  <section className="section" id="capabilities">
    <div className="container">
      <div className="section-head">
        <div>
          <div className="section-kicker">Возможности</div>
          <h2 className="section-title">Что вы получаете внутри.</h2>
        </div>
        <p className="section-lede">
          Базовая математика, которая всегда работает одинаково, плюс AI поверх — чтобы не считать
          рыночную цену пальцем по воздуху.
        </p>
      </div>
      <div className="cap-grid">
        {CAPS.map((c) => (
          <div className="cap" key={c.ix}>
            <div className="cap-ix">{c.ix}</div>
            <div className="cap-icon" aria-hidden="true" />
            <h3 className="cap-title">{c.t}</h3>
            <p className="cap-desc">{c.d}</p>
          </div>
        ))}
      </div>
    </div>
  </section>
);

export default SolutionsSection;
