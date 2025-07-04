export async function fetchData(url) {
    try {
        const response = await fetch(url);
        if (!response.ok) {
            const errorData = await response.json().catch(() => ({}));
            throw new Error(errorData.error || `HTTP error! Status: ${response.status}`);
        }
        return await response.json();
    } catch (error) {
        console.error('Fetch error:', error);
        throw error;
    }
}

export async function fetchVideos(API_BASE_URL, perPage, VIDEO_LOADING, VIDEO_LIST, VIDEO_ERROR, VIDEO_PAGINATION, fetchData, renderVideoList, renderPagination, showLoading, showError, hideLoading, page, search, setTotalVideoPages, setCurrentVideoPage) {
    showLoading(VIDEO_LOADING, VIDEO_LIST, VIDEO_ERROR);
    VIDEO_PAGINATION.innerHTML = '';
    try {
        const url = `${API_BASE_URL}/api/videos?page=${page}&pageSize=${perPage}&search=${encodeURIComponent(search)}`;
        const response = await fetchData(url);
        if (response && Array.isArray(response.videos)) {
            renderVideoList(response.videos);
            const total = response.total || 0;
            const currentPage = response.page || page;
            const pageSize = response.page_size || perPage;
            const totalPages = Math.ceil(total / pageSize);
            const pagination = {
                total_pages: totalPages,
                current_page: currentPage,
                total_items: total,
                has_next: currentPage < totalPages,
                has_prev: currentPage > 1
            };
            renderPagination(VIDEO_PAGINATION, pagination, (newPage) => fetchVideos(API_BASE_URL, perPage, VIDEO_LOADING, VIDEO_LIST, VIDEO_ERROR, VIDEO_PAGINATION, fetchData, renderVideoList, renderPagination, showLoading, showError, hideLoading, newPage, search, setTotalVideoPages, setCurrentVideoPage), search);
            setTotalVideoPages(totalPages);
            setCurrentVideoPage(currentPage);
            if (response.videos.length === 0) {
                showError(VIDEO_ERROR, VIDEO_LIST, search ? '没有找到匹配的视频' : '数据库中没有视频');
            }
        } else {
            const errorMsg = `无效的API响应格式，videos字段应为数组。实际返回: ${JSON.stringify(response)}`;
            throw new Error(errorMsg);
        }
    } catch (error) {
        console.error('Fetch Videos Error:', error);
        showError(VIDEO_ERROR, VIDEO_LIST, `加载视频失败: ${error.message}`);
    } finally {
        hideLoading(VIDEO_LOADING);
    }
}

export async function fetchComments(API_BASE_URL, commentsPerPage, COMMENT_LOADING, COMMENT_LIST, COMMENT_ERROR, COMMENT_PAGINATION, fetchData, fetchCommentReplies, renderCommentList, renderPagination, showLoading, showError, hideLoading, selectedBvid, page, keyword, setTotalCommentPages, setCurrentCommentPage, COMMENT_COUNT) {
    if (!selectedBvid) return;
    showLoading(COMMENT_LOADING, COMMENT_LIST, COMMENT_ERROR);
    COMMENT_PAGINATION.innerHTML = '';
    try {
        let url = `${API_BASE_URL}/api/comments/${selectedBvid}?page=${page}&pageSize=${commentsPerPage}`;
        if (keyword) {
            url += `&keyword=${encodeURIComponent(keyword)}`;
        }
        const response = await fetchData(url);
        if (response && response.comments) {
            COMMENT_COUNT.textContent = response.total;
            const commentsWithReplies = await Promise.all(
                response.comments.map(async comment => {
                    try {
                        const replyData = await fetchCommentReplies(API_BASE_URL, comment.unique_id, 1, fetchData, 5);
                        if (replyData) {
                            comment.loadedReplies = replyData.replies || [];
                            comment.totalReplies = replyData.total || 0;
                            comment.currentReplyPage = 1;
                        }
                    } catch (error) {
                        console.error('自动加载回复失败:', error);
                        comment.loadedReplies = [];
                        comment.totalReplies = 0;
                    }
                    return comment;
                })
            );
            renderCommentList(commentsWithReplies);
            const total = response.total || 0;
            const totalPages = Math.ceil(total / commentsPerPage);
            const pagination = {
                total_pages: totalPages,
                current_page: page,
                total_items: total,
                has_next: page < totalPages,
                has_prev: page > 1
            };
            renderPagination(COMMENT_PAGINATION, pagination, (newPage) => fetchComments(API_BASE_URL, commentsPerPage, COMMENT_LOADING, COMMENT_LIST, COMMENT_ERROR, COMMENT_PAGINATION, fetchData, fetchCommentReplies, renderCommentList, renderPagination, showLoading, showError, hideLoading, selectedBvid, newPage, keyword, setTotalCommentPages, setCurrentCommentPage, COMMENT_COUNT), keyword);
            setTotalCommentPages(totalPages);
            setCurrentCommentPage(page);
            if (response.comments.length === 0) {
                const msg = keyword ? `没有找到匹配"${keyword}"的评论` : '该视频暂无评论';
                showError(COMMENT_ERROR, COMMENT_LIST, msg);
            }
        } else {
            throw new Error('无效的API响应格式');
        }
    } catch (error) {
        showError(COMMENT_ERROR, COMMENT_LIST, `加载评论失败: ${error.message}`);
    } finally {
        hideLoading(COMMENT_LOADING);
    }
}

export async function fetchCommentReplies(API_BASE_URL, commentId, page, fetchData, repliesPerPage = 5) {
    try {
        const url = `${API_BASE_URL}/api/comment/replies/${commentId}?page=${page}&pageSize=${repliesPerPage}`;
        const response = await fetchData(url);
        if (response && response.replies) {
            return {
                replies: response.replies,
                total: response.total,
                page: page,
                pageSize: repliesPerPage
            };
        } else {
            throw new Error('无效的回复API响应格式');
        }
    } catch (error) {
        console.error('获取评论回复失败:', error);
        return null;
    }
} 