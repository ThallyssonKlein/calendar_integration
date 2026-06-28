var CRUD_API_URL = 'https://YOUR_CRUD_API_URL'; // e.g. https://calendar-api.vercel.app
var API_TOKEN = 'YOUR_TOKEN_HERE';

function syncCalendarEvents() {
  var now = new Date();

  var startDate = new Date(now);
  startDate.setDate(startDate.getDate() - 30);

  var endDate = new Date(now);
  endDate.setDate(endDate.getDate() + 30);

  var calendars = CalendarApp.getAllCalendars();
  var allGoogleEvents = [];

  for (var i = 0; i < calendars.length; i++) {
    var cal = calendars[i];
    var evts = cal.getEvents(startDate, endDate);
    allGoogleEvents = allGoogleEvents.concat(evts);
  }

  if (allGoogleEvents.length === 0) {
    Logger.log('No events found in range.');
    return;
  }

  var tz = Session.getScriptTimeZone();
  var seen = {};
  var events = [];

  for (var i = 0; i < allGoogleEvents.length; i++) {
    var e = allGoogleEvents[i];
    var eventId = e.getId();
    if (seen[eventId]) continue;
    seen[eventId] = true;

    var event = { id: eventId };

    if (e.isAllDayEvent()) {
      event.start = { date: Utilities.formatDate(e.getStartTime(), tz, 'yyyy-MM-dd') };
      event.end   = { date: Utilities.formatDate(e.getEndTime(),   tz, 'yyyy-MM-dd') };
    } else {
      event.start = { dateTime: e.getStartTime().toISOString(), timeZone: tz };
      event.end   = { dateTime: e.getEndTime().toISOString(),   timeZone: tz };
    }

    var title = e.getTitle();
    if (title) event.summary = title;

    var desc = e.getDescription();
    if (desc) event.description = desc;

    var loc = e.getLocation();
    if (loc) event.location = loc;

    var guests = e.getGuestList(true);
    if (guests.length > 0) {
      event.attendees = guests.map(function(g) {
        return {
          email: g.getEmail(),
          displayName: g.getName() || '',
          responseStatus: mapGuestStatus(g.getGuestStatus())
        };
      }).sort(function(a, b) { return a.email < b.email ? -1 : 1; });
    }

    events.push(event);
  }

  var baseUrl = CRUD_API_URL.replace(/\/+$/, '');
  var response = UrlFetchApp.fetch(baseUrl + '/events', {
    method: 'post',
    contentType: 'application/json',
    headers: {
      'Authorization': 'Bearer ' + API_TOKEN
    },
    payload: JSON.stringify(events),
    muteHttpExceptions: true,
    followRedirects: false
  });

  Logger.log('Status: ' + response.getResponseCode());
  Logger.log('Response: ' + response.getContentText());
}

function mapGuestStatus(status) {
  switch (status) {
    case CalendarApp.GuestStatus.YES:    return 'accepted';
    case CalendarApp.GuestStatus.NO:     return 'declined';
    case CalendarApp.GuestStatus.MAYBE:  return 'tentative';
    default:                             return 'needsAction';
  }
}
