document.addEventListener('keydown', (event) => {
  if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
    const form = event.target && event.target.closest && event.target.closest('form');
    if (form) {
      event.preventDefault();
      if (window.htmx) window.htmx.trigger(form, 'submit'); else form.requestSubmit();
    }
  }
});

document.body.addEventListener('htmx:afterSwap', (event) => {
  const toastRegion = document.getElementById('toast-region');
  const notice = event.detail.target && event.detail.target.querySelector && event.detail.target.querySelector('.notice');
  if (toastRegion && notice && event.detail.target.id !== 'toast-region') {
    const toast = document.createElement('div');
    toast.className = 'toast';
    toast.textContent = notice.textContent;
    toastRegion.replaceChildren(toast);
    setTimeout(() => toast.remove(), 3500);
  }
});
