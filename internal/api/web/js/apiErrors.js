// Shared helper for turning a failed fetch() response into a short,
// readable message - every page's error handling was building its Error
// straight from the raw response body, which is fine for comic-server's
// own http.Error() responses (short plain text) but breaks badly when a
// reverse proxy in front of the server answers instead: a 504 Gateway
// Timeout from nginx/openresty is a full HTML document, and that whole
// page was landing verbatim in the UI's error text/toast.
//
// bodyText is the already-read response body (callers already need it to
// decide whether the response was JSON on success, so this never re-reads
// the body itself). Returns bodyText unchanged when it looks like a short,
// genuine server message; otherwise a generic message built from the HTTP
// status.
function friendlyErrorText(response, bodyText, fallback) {
    const trimmed = (bodyText || '').trim();
    const looksLikeHTML = /^<(!doctype|html)/i.test(trimmed);
    const tooLong = trimmed.length > 500;

    if (!trimmed) {
        return fallback || `HTTP ${response.status}`;
    }
    if (looksLikeHTML || tooLong) {
        if (response.status === 504) {
            return 'The server took too long to respond (504 Gateway Timeout). This can happen on a large library - try again, or check the server logs if it keeps happening.';
        }
        return `Server error (HTTP ${response.status}). Try again in a moment.`;
    }
    return trimmed;
}

window.friendlyErrorText = friendlyErrorText;
