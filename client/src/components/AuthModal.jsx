const AuthModal = ({ isOpen, onClose }) => {
  if (!isOpen) {
    return null;
  }

  const handleLogin = () => {
    const clientId = import.meta.env.VITE_YA_CLIENT_ID;
    if (!clientId) {
      console.log("Не задан VITE_YA_CLIENT_ID для Яндекс ID.");
      return;
    }

    const redirectUri =
      import.meta.env.VITE_YA_REDIRECT_URI || `${window.location.origin}/auth`;
    const params = new URLSearchParams({
      client_id: clientId,
      response_type: "code",
      redirect_uri: redirectUri,
    });

    window.location.assign(`https://oauth.yandex.ru/authorize?${params.toString()}`);
  };

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
        <button className="btn btn--primary modal__action" type="button" onClick={handleLogin}>
          Вход
        </button>
      </div>
    </div>
  );
};

export default AuthModal;
