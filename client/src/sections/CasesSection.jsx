const CasesSection = () => (
  <section className="section" id="example">
    <div className="container">
      <div className="section-head">
        <div>
          <div className="section-kicker">Пример · 15 пиджаков</div>
          <h2 className="section-title">Как раньше — и как с PoshivOn.</h2>
        </div>
        <p className="section-lede">
          Реальный кейс ателье на 12 человек: серия мужских пиджаков из костюмной ткани.
          Раньше технолог считал заказ в Excel, теперь — в панели.
        </p>
      </div>
      <div className="ba-grid">
        <article className="ba-card before">
          <div className="ba-label">
            <span className="pip" />
            Раньше · Excel
          </div>
          <h3 className="ba-h">Таблица на две вкладки и созвон с закройщиком</h3>
          <div className="ba-list">
            <div className="ba-item"><span className="mk">✕</span>Переписывание норм при каждом тираже</div>
            <div className="ba-item"><span className="mk">✕</span>Расхождения с закройщиком по операциям</div>
            <div className="ba-item"><span className="mk">✕</span>Коммерческое считается полдня</div>
            <div className="ba-item"><span className="mk">✕</span>Цена на глаз, без опоры на рынок</div>
          </div>
          <div className="ba-metrics">
            <div className="ba-metric"><div className="n">2 ч</div><div className="l">на расчёт</div></div>
            <div className="ba-metric"><div className="n">±15%</div><div className="l">погрешность</div></div>
            <div className="ba-metric"><div className="n">—</div><div className="l">AI-оценка</div></div>
          </div>
        </article>
        <article className="ba-card after">
          <div className="ba-label">
            <span className="pip" />
            С PoshivOn
          </div>
          <h3 className="ba-h">Одна форма, общий справочник, итог с AI-оценкой</h3>
          <div className="ba-list">
            <div className="ba-item"><span className="mk">✓</span>Нормы задаются один раз, подставляются всегда</div>
            <div className="ba-item"><span className="mk">✓</span>Закройщик и руководитель видят одно и то же</div>
            <div className="ba-item"><span className="mk">✓</span>Коммерческое — через две минуты после вводных</div>
            <div className="ba-item"><span className="mk">✓</span>AI подсказывает сегмент и диапазон</div>
          </div>
          <div className="ba-metrics">
            <div className="ba-metric"><div className="n">2 мин</div><div className="l">на расчёт</div></div>
            <div className="ba-metric"><div className="n">±2%</div><div className="l">погрешность</div></div>
            <div className="ba-metric"><div className="n">✓</div><div className="l">AI-оценка</div></div>
          </div>
        </article>
      </div>
    </div>
  </section>
);

export default CasesSection;
