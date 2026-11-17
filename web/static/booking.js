// Books a lab without reloading the page (AC-4). Without this script the Book form still posts
// normally and the server redirects or shows the error page, so nothing depends on it.
(function () {
  'use strict';

  // The server's sentence for a full lab (service.ErrLabFull).
  var FULL = 'This lab is full.';
  var flash = document.getElementById('flash');
  // The signed-in session's CSRF token, sent with every booking (AC-87).
  var tokenMeta = document.querySelector('meta[name="csrf-token"]');
  var token = tokenMeta ? tokenMeta.getAttribute('content') : '';

  function link(href, text) {
    var a = document.createElement('a');
    a.href = href;
    a.textContent = text;
    return a;
  }

  // show puts one sentence in the message area at the top of the page and moves focus there, so
  // keyboard and screen-reader users hear it too.
  function show(text, ok) {
    if (!flash) return;
    flash.textContent = text;
    flash.className = 'message ' + (ok ? 'message--success' : 'message--error');
    flash.setAttribute('role', ok ? 'status' : 'alert');
    if (!ok) {
      // AC-1: the way back to the lab list and the home page.
      flash.append(' ', link('/labs', 'Back to the lab list'), ' · ', link('/', 'Home page'));
    }
    flash.hidden = false;
    flash.tabIndex = -1;
    flash.focus();
  }

  function fallback(status) {
    return status === 401 ? 'Please sign in to continue.' : 'Something went wrong on our side. Please try again.';
  }

  document.addEventListener('submit', function (event) {
    var form = event.target;
    if (!form.classList || !form.classList.contains('book-form')) return;
    event.preventDefault();

    var button = form.querySelector('button');
    var card = form.closest('[data-session]');
    var label = button.textContent;
    button.disabled = true;
    button.textContent = 'Booking…';

    fetch(form.action, { method: 'POST', headers: { Accept: 'application/json', 'X-CSRF-Token': token }, credentials: 'same-origin' })
      .then(function (res) {
        return res.json()
          .catch(function () { return {}; })
          .then(function (body) { return { status: res.status, ok: res.ok, body: body }; });
      })
      .then(function (r) {
        if (r.ok) {
          var places = card && card.querySelector('[data-free-places]');
          if (places) places.textContent = r.body.free_places + ' of ' + r.body.place_limit + ' places free';
          button.textContent = 'Booked';
          show(r.body.message || 'Booking confirmed.', true);
          return;
        }
        if (r.status === 409) {
          // Full, already booked or not open today: trying again cannot help.
          button.textContent = r.body.message === FULL ? 'Full' : label;
        } else {
          button.disabled = false;
          button.textContent = label;
        }
        show(r.body.message || fallback(r.status), false);
      })
      .catch(function () {
        button.disabled = false;
        button.textContent = label;
        show('The booking could not be sent. Please check your connection and try again.', false);
      });
  });
})();
