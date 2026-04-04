const FeaturesSection = ({ items }) => (
  <section className="section" id="about">
    <div className="container">
      <h2>Для кого это</h2>
      <div className="feature-grid">
        {items.map((item) => (
          <div className="feature-card" key={item.title}>
            <h3>{item.title}</h3>
            <p>{item.description}</p>
          </div>
        ))}
      </div>
    </div>
  </section>
);

export default FeaturesSection;
