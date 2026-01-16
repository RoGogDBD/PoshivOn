const CasesSection = ({ items }) => (
  <section className="section" id="cases">
    <div className="container">
      <h2>Решения от PoshivOn</h2>
      <div className="case-grid">
        <div className="case-preview">скрин</div>
        <div className="case-list">
          {items.map((item) => (
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
          {items.map((item) => (
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
);

export default CasesSection;
