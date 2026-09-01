import { useEffect, useMemo, useState } from "react";
import { QUIZ_STEPS, QUIZ_START, calculateEstimate } from "../data/demoQuiz.js";

const RUB_FORMAT = new Intl.NumberFormat("ru-RU", { style: "currency", currency: "RUB", maximumFractionDigits: 0 });

// Счёт от 0 до итоговой суммы — редкий, "финальный" момент за всю сессию (см. animate:
// частота "rare/first-time" — это как раз бюджет на delight), поэтому анимация оправдана
// и может быть чуть длиннее обычных UI-переходов. Это не transform/opacity (тут число —
// текст), поэтому CSS-transition не подходит: считаем сами через requestAnimationFrame,
// с ручным сильным ease-out (старт быстрый, торможение к финалу — по духу тот же
// --ease-out: cubic-bezier(0.23, 1, 0.32, 1), только для числового тика, а не для transform).
// prefers-reduced-motion — не пропускаем анимацию совсем, а сразу показываем итог.
function useCountUp(target, duration = 900) {
  const [value, setValue] = useState(0);
  useEffect(() => {
    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (reduceMotion || target <= 0) {
      setValue(target);
      return;
    }
    let raf;
    const start = performance.now();
    const tick = (now) => {
      const t = Math.min(1, (now - start) / duration);
      const eased = 1 - Math.pow(1 - t, 4);
      setValue(Math.round(target * eased));
      if (t < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [target, duration]);
  return value;
}

// Отдельный компонент — чтобы хук считал именно от 0 при каждом попадании на экран
// результата (монтируется/размонтируется вместе с ним), а не при каждом ререндере DemoQuiz.
const PriceEstimate = ({ amount }) => {
  const animated = useCountUp(amount);
  return (
    <div className="demo-quiz__price">
      <div className="demo-quiz__price-label">Примерная стоимость</div>
      <div className="demo-quiz__price-value">{RUB_FORMAT.format(animated)}</div>
      <p className="demo-quiz__price-note">Ориентир по вашим ответам — точную цену назовём после уточнения деталей.</p>
    </div>
  );
};

// Дерево (data/demoQuiz.js) не линейно и не двоичное: "multi"-шаг может открыть несколько
// followUp-шагов подряд (по одному на каждую отмеченную деталь — например "Разрез" и
// "Молния" оба выбраны → сперва "Сколько разрезов?", потом "Сколько молний?"), а некоторые
// followUp сами являются multi-шагом со своими followUp-ами (ветка "Платье": "Карман" →
// вид карманов → счётчик на каждый выбранный вид). Поэтому вместо плоского history[] нужен
// стек "кадров" — на каждом шаге храним не только его id, но и очередь ещё не показанных
// followUp-ов этого шага (queue) и то, куда идти, когда очередь опустеет (queueNext).
//
// resolveNext — единственное место, где эта очередь считается, и работает одинаково для
// всех трёх типов ответа: у single ничего не добавляется в очередь (followIds = []),
// у multi — в начало очереди встают followUp-ы отмеченных вариантов (в порядке списка
// options), у number/text — тоже [] (сами они бывают только внутри чужой очереди или как
// простой next). "Назад" — просто откат стека кадров, поэтому очередь восстанавливается
// автоматически, без отдельной логики отмены.
function resolveNext(frame, followIds, ownNext) {
  const queueAhead = [...followIds, ...frame.queue];
  if (queueAhead.length > 0) {
    return { stepId: queueAhead[0], queue: queueAhead.slice(1), queueNext: frame.queueNext ?? ownNext };
  }
  return { stepId: frame.queueNext ?? ownNext, queue: [], queueNext: null };
}

const INITIAL_FRAME = { stepId: QUIZ_START, queue: [], queueNext: null };

// По итогам опроса виджет показывает примерную стоимость (расчёт — calculateEstimate
// в data/demoQuiz.js, счётчик с анимацией — PriceEstimate ниже) и собирает контакты для
// точного расчёта. Сама отправка контактов пока никуда не подключена — черновик для
// локального согласования.
const DemoQuiz = () => {
  const [history, setHistory] = useState([INITIAL_FRAME]);
  const [answers, setAnswers] = useState({});
  const [contact, setContact] = useState({ name: "", phone: "" });
  const [isSubmitted, setIsSubmitted] = useState(false);

  const frame = history[history.length - 1];
  const isResult = frame.stepId === "result";
  const step = isResult ? null : QUIZ_STEPS[frame.stepId];
  const estimate = useMemo(() => calculateEstimate(answers), [answers]);

  // Точное "из скольки" неизвестно заранее — ветки multi-шагов раскрываются по ходу
  // ответа (см. комментарий выше). Поэтому общее число — честная оценка "того, что уже
  // видно": пройденные шаги + то, что уже стоит в очереди + ещё один шаг за горизонтом.
  // Она растёт и уменьшается вместе с реальной веткой и всегда сходится к 100% на "result".
  const totalSteps = isResult ? history.length : history.length + frame.queue.length + 1;
  const progressPercent = Math.min(100, Math.round((history.length / totalSteps) * 100));

  const pushFrame = (nextFrame) => setHistory((prev) => [...prev, nextFrame]);

  const handleSingleSelect = (option) => {
    setAnswers((prev) => ({ ...prev, [frame.stepId]: option }));
    pushFrame(resolveNext(frame, [], option.next));
  };

  const handleMultiSubmit = (selectedValues) => {
    const selectedOptions = step.options.filter((option) => selectedValues.includes(option.value));
    setAnswers((prev) => ({ ...prev, [frame.stepId]: selectedOptions }));
    const followIds = selectedOptions.filter((option) => option.followUp).map((option) => option.followUp);
    pushFrame(resolveNext(frame, followIds, step.next));
  };

  const handleValueSubmit = (value) => {
    setAnswers((prev) => ({ ...prev, [frame.stepId]: value }));
    pushFrame(resolveNext(frame, [], step.next));
  };

  const handleBack = () => {
    setHistory((prev) => (prev.length > 1 ? prev.slice(0, -1) : prev));
  };

  const handleRestart = () => {
    setHistory([INITIAL_FRAME]);
    setAnswers({});
    setContact({ name: "", phone: "" });
    setIsSubmitted(false);
  };

  const handleContactSubmit = (event) => {
    event.preventDefault();
    setIsSubmitted(true);
  };

  return (
    <div className="demo-quiz">
      <div
        className="demo-quiz__progress"
        role="progressbar"
        aria-valuenow={progressPercent}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label="Прогресс опроса"
      >
        <div className="demo-quiz__progress-fill" style={{ width: `${progressPercent}%` }} />
      </div>

      {!isResult && step && (
        <div className="demo-quiz__card" key={step.id}>
          <div className="demo-quiz__step-label">
            Вопрос {history.length} из {totalSteps}
          </div>
          <h3 className="demo-quiz__question">{step.question}</h3>
          {step.subtitle && <p className="demo-quiz__subtitle">{step.subtitle}</p>}

          {step.type === "single" && (
            <div className="demo-quiz__options">
              {step.options.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className="demo-quiz__option"
                  onClick={() => handleSingleSelect(option)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          )}

          {step.type === "multi" && <MultiStep step={step} onSubmit={handleMultiSubmit} />}
          {step.type === "number" && <NumberStep onSubmit={handleValueSubmit} />}
          {step.type === "text" && <TextStep onSubmit={handleValueSubmit} />}

          {history.length > 1 && (
            <button type="button" className="demo-quiz__back" onClick={handleBack}>
              ← Назад
            </button>
          )}
        </div>
      )}

      {isResult && !isSubmitted && (
        <div className="demo-quiz__card demo-quiz__result">
          <div className="demo-quiz__step-label">Готово</div>
          <h3 className="demo-quiz__question">Заявка создана</h3>
          <p className="demo-quiz__subtitle">
            Осталось оставить контактные данные — и с вами свяжутся, чтобы уточнить детали и
            посчитать точную стоимость.
          </p>
          <PriceEstimate amount={estimate} />
          <form className="demo-quiz__form" onSubmit={handleContactSubmit}>
            <input
              className="demo-quiz__input"
              type="text"
              name="name"
              placeholder="Имя"
              autoComplete="name"
              required
              value={contact.name}
              onChange={(event) => setContact((prev) => ({ ...prev, name: event.target.value }))}
            />
            <input
              className="demo-quiz__input"
              type="tel"
              name="phone"
              placeholder="+7 (___) ___-__-__"
              autoComplete="tel"
              required
              value={contact.phone}
              onChange={(event) => setContact((prev) => ({ ...prev, phone: event.target.value }))}
            />
            <button type="submit" className="btn btn-primary btn-lg demo-quiz__submit">
              Отправить заявку
            </button>
          </form>
          <button type="button" className="demo-quiz__back" onClick={handleRestart}>
            ← Пройти ещё раз
          </button>
        </div>
      )}

      {isResult && isSubmitted && (
        <div className="demo-quiz__card demo-quiz__result demo-quiz__result--success">
          <div className="demo-quiz__step-label">Спасибо</div>
          <h3 className="demo-quiz__question">Заявка отправлена</h3>
          <p className="demo-quiz__subtitle">
            Мы получили ваши контакты и скоро свяжемся, чтобы уточнить детали и посчитать точную
            стоимость.
          </p>
          <div className="demo-quiz__result-actions">
            <button type="button" className="btn btn-outline" onClick={handleRestart}>
              Пройти ещё раз
            </button>
          </div>
        </div>
      )}
    </div>
  );
};

// Своё состояние выбранных чекбоксов, независимое от родителя — родитель ремонтирует этот
// компонент через key={step.id} на карточке-обёртке, так что при переходе на новый шаг
// выбор всегда стартует пустым.
const MultiStep = ({ step, onSubmit }) => {
  const [selected, setSelected] = useState([]);

  const toggle = (value) => {
    setSelected((prev) => (prev.includes(value) ? prev.filter((v) => v !== value) : [...prev, value]));
  };

  return (
    <>
      <div className="demo-quiz__options">
        {step.options.map((option) => {
          const isSelected = selected.includes(option.value);
          return (
            <button
              key={option.value}
              type="button"
              className={`demo-quiz__option${isSelected ? " is-selected" : ""}`}
              aria-pressed={isSelected}
              onClick={() => toggle(option.value)}
            >
              {option.label}
            </button>
          );
        })}
      </div>
      <button
        type="button"
        className="btn btn-primary btn-lg demo-quiz__submit"
        disabled={selected.length === 0}
        onClick={() => onSubmit(selected)}
      >
        Далее
      </button>
    </>
  );
};

const NumberStep = ({ onSubmit }) => {
  const [value, setValue] = useState("");

  const handleSubmit = (event) => {
    event.preventDefault();
    onSubmit(value);
  };

  return (
    <form className="demo-quiz__form" onSubmit={handleSubmit}>
      <input
        className="demo-quiz__input"
        type="number"
        inputMode="numeric"
        min="0"
        placeholder="Количество"
        required
        value={value}
        onChange={(event) => setValue(event.target.value)}
        autoFocus
      />
      <button type="submit" className="btn btn-primary btn-lg demo-quiz__submit">
        Далее
      </button>
    </form>
  );
};

const TextStep = ({ onSubmit }) => {
  const [value, setValue] = useState("");

  const handleSubmit = (event) => {
    event.preventDefault();
    onSubmit(value);
  };

  return (
    <form className="demo-quiz__form" onSubmit={handleSubmit}>
      <input
        className="demo-quiz__input"
        type="text"
        placeholder="Опишите, что нужно"
        required
        value={value}
        onChange={(event) => setValue(event.target.value)}
        autoFocus
      />
      <button type="submit" className="btn btn-primary btn-lg demo-quiz__submit">
        Далее
      </button>
    </form>
  );
};

export default DemoQuiz;
