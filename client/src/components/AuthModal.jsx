const AuthModal = ({ isOpen, onClose }) => {
  if (!isOpen) {
    return null;
  }

  return (
    <div
      className="modal-overlay"
      role="presentation"
      onClick={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="auth-title">
        <button className="modal__close" type="button" onClick={onClose} aria-label="Закрыть окно входа">
          ×
        </button>
        <h3 id="auth-title">Вход через Яндекс ID</h3>
        <p className="modal__action">Откроется окно мгновенной авторизации Яндекс ID.</p>
      </div>
    </div>
  );
};

export default AuthModal;
