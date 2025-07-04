export function openModal(imageUrl, MODAL_IMAGE, IMAGE_MODAL) {
    MODAL_IMAGE.src = imageUrl;
    IMAGE_MODAL.classList.add('visible');
    document.body.style.overflow = 'hidden';
}

export function closeModal(MODAL_IMAGE, IMAGE_MODAL) {
    IMAGE_MODAL.classList.remove('visible');
    MODAL_IMAGE.src = '';
    document.body.style.overflow = '';
}

export function bindModalEvents(MODAL_CLOSE, IMAGE_MODAL, MODAL_IMAGE, closeModal) {
    MODAL_CLOSE.addEventListener('click', () => closeModal(MODAL_IMAGE, IMAGE_MODAL));
    IMAGE_MODAL.addEventListener('click', (e) => {
        if (e.target === IMAGE_MODAL) closeModal(MODAL_IMAGE, IMAGE_MODAL);
    });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && IMAGE_MODAL.classList.contains('visible')) {
            closeModal(MODAL_IMAGE, IMAGE_MODAL);
        }
    });
} 