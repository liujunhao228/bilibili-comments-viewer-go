export function bindEvents({
    BACK_BUTTON,
    SEARCH_BUTTON,
    SEARCH_INPUT,
    COMMENT_SEARCH_BUTTON,
    COMMENT_SEARCH_INPUT,
    VIDEO_COVER,
    COMMENT_LIST,
    VIDEO_LIST_VIEW,
    COMMENT_VIEW,
    VIDEO_LIST,
    VIDEO_PAGINATION,
    VIDEO_LOADING,
    VIDEO_ERROR,
    COMMENT_PAGINATION,
    COMMENT_LOADING,
    COMMENT_ERROR,
    VIDEO_TITLE,
    COMMENT_COUNT,
    MODAL_IMAGE,
    IMAGE_MODAL,
    MODAL_CLOSE,
    THEME_TOGGLE,
    fetchVideos,
    fetchComments,
    handleVideoClick,
    handleLoadMoreReplies,
    openModal,
    closeModal,
    loadedReplies,
    setCurrentVideoPage,
    setCurrentCommentPage,
    setCurrentSearchTerm,
    setCurrentCommentSearchTerm,
    setSelectedBvid,
    setTotalVideoPages,
    setTotalCommentPages
}) {
    BACK_BUTTON.addEventListener('click', () => {
        COMMENT_VIEW.style.display = 'none';
        VIDEO_LIST_VIEW.style.display = 'block';
        setSelectedBvid(null);
        window.scrollTo(0, 0);
        COMMENT_LIST.innerHTML = '';
        COMMENT_PAGINATION.innerHTML = '';
        COMMENT_ERROR.style.display = 'none';
        COMMENT_ERROR.textContent = '';
        VIDEO_TITLE.textContent = '';
        VIDEO_COVER.src = '';
        VIDEO_COVER.alt = '';
        COMMENT_COUNT.textContent = '0';
        COMMENT_SEARCH_INPUT.value = '';
        loadedReplies.clear();
    });
    SEARCH_BUTTON.addEventListener('click', () => {
        setCurrentSearchTerm(SEARCH_INPUT.value.trim());
        setCurrentVideoPage(1);
        fetchVideos();
    });
    SEARCH_INPUT.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
            setCurrentSearchTerm(SEARCH_INPUT.value.trim());
            setCurrentVideoPage(1);
            fetchVideos();
        }
    });
    COMMENT_SEARCH_BUTTON.addEventListener('click', () => {
        setCurrentCommentSearchTerm(COMMENT_SEARCH_INPUT.value.trim());
        setCurrentCommentPage(1);
        fetchComments();
    });
    COMMENT_SEARCH_INPUT.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
            setCurrentCommentSearchTerm(COMMENT_SEARCH_INPUT.value.trim());
            setCurrentCommentPage(1);
            fetchComments();
        }
    });
    VIDEO_COVER.addEventListener('click', () => {
        if (VIDEO_COVER.src) openModal(VIDEO_COVER.src);
    });
    COMMENT_LIST.addEventListener('click', (e) => {
        if (e.target.tagName === 'IMG' && e.target.closest('.comment-pictures')) {
            if (e.target.src) openModal(e.target.src);
        }
    });
} 