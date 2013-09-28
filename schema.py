from collections import defaultdict
import datetime
import logging
import random
from utils import Utils

from google.appengine.ext import ndb
from oauth2client.appengine import CredentialsNDBProperty

class Gamenight(ndb.Model):
    """Gamenights that have been scheduled."""
    invitation = ndb.KeyProperty('a', kind='Invitation')
    event = ndb.StringProperty('e')
    status = ndb.StringProperty('s', choices=['Yes', 'Probably', 'Maybe', 'No'])
    lastupdate = ndb.DateTimeProperty('u')

    # denormalized from invitation for datastore efficiency
    date = ndb.DateProperty('d')
    time = ndb.TimeProperty('t')
    owner = ndb.KeyProperty('o', kind='User')
    location = ndb.StringProperty('l', indexed=False)
    notes = ndb.StringProperty('n', indexed=False)

    def _pre_put_hook(self):
        creds = Auth.get()
        if not creds:
            logging.warn('No credentials found, event not updated.')
            return

        service = Utils.get_service(Auth)
        calendar_id = self._get_config('calendar_id')
        if self.event:
            logging.info('Updating event for %s.', self.date)
            event = service.events().get(calendarId=calendar_id,
                                         eventId=self.event).execute()
            event.update(self._make_event())
            service.events().update(calendarId=calendar_id,
                                    eventId=self.event,
                                    body=event).execute()
        else:
            logging.info('Creating new event for %s.', self.date)
            event = self._make_event()
            newevent = service.events().insert(calendarId=calendar_id,
                                                   body=event).execute()
            self.event = newevent['id']
            logging.info('New event: %r.', newevent)

    @classmethod
    def _pre_delete_hook(cls, key):
        gn = key.get()
        eventid = gn.event
        if not eventid:
            return

        service = Utils.get_service(Auth)
        calendar_id = gn._get_config('calendar_id')
        logging.info('Deleting event: %s', eventid)
        service.events().delete(
            calendarId=calendar_id, eventId=eventid).execute()

    def _get_config(self, key):
        conf = Config.query(Config.name==key).get()
        if conf:
            return conf.value
        else:
            return None

    def _make_event(self):
        if not self.time:
            self.time = datetime.time(20, 0, 0)

        start = datetime.datetime.combine(self.date, self.time)
        event = {
            'start': { 'dateTime': start.strftime('%Y-%m-%dT%H:%M:%S'),
                     'timeZone': 'America/Los_Angeles' },
            'end': { 'dateTime': self.date.strftime('%Y-%m-%dT23:59:59'),
                     'timeZone': 'America/Los_Angeles' },
            'description': self.notes,
            'location': self.location,
            'summary': 'Gamenight: %s' % self.status.upper(),
        }

        return event

    @classmethod
    def schedule(cls, date=None, fallback='Probably'):
        if date is None:
            date = Utils.saturday()

        schedule = cls.query(cls.date==date).filter(cls.status=='Yes').get() or \
                   Invitation.resolve(when=date) or \
                   cls.query(cls.date==date).get() or \
                   Gamenight(status=fallback,
                             date=date,
                             lastupdate=Utils.now())
        schedule.put()
        logging.info('Scheduling new gamenight: %r', schedule)

        return schedule

    @classmethod
    def future(cls, limit):
        return cls.query(cls.date >= Utils.now()).order(cls.date).fetch(limit)

    @classmethod
    def this_week(cls):
        """Get this week's gamenight."""

        g = cls.future(1)
        if g and g[0].is_this_week():
            return g[0]
        else:
            return None

    def update(self):
        if not self.invitation:
            return False

        invite = self.invitation.get()
        self.time = invite.time
        self.location = invite.location
        self.notes = invite.notes
        self.put()

        return True


    def is_this_week(self):
        return self.date - datetime.date.today() < datetime.timedelta(7)


class Invitation(ndb.Model):
    """Entries of offers to host."""
    date = ndb.DateProperty('d')
    time = ndb.TimeProperty('t')
    owner = ndb.KeyProperty('o', kind='User')
    location = ndb.StringProperty('l', indexed=False)
    notes = ndb.StringProperty('n', indexed=False)
    priority = ndb.StringProperty('p', choices=['Can', 'Want', 'Insist'])

    def text_date(self):
        date = datetime.datetime.combine(self.date, self.time)
        if self.date == datetime.date.today():
            return self.time.strftime('Today, %I:%M %p')
        if date - Utils.now() < datetime.timedelta(6):
            return self.time.strftime('Saturday, %I:%M %p')
        return date.strftime('%b %d, %I:%M %p')

    datetext = ndb.ComputedProperty(text_date)

    text_pri = { 'Can': 'Could host',
                 'Want': 'Want to host',
                 'Insist': 'Would really want to host' }
    priority_text = ndb.ComputedProperty(lambda self: self.text_pri[self.priority])

    @classmethod
    def get(cls, key):
        return ndb.Key(cls, 'root', cls, int(key)).get()

    @classmethod
    def dummy(cls):
        return ndb.Key(cls, 'root')

    @classmethod
    def resolve(cls, when=Utils.saturday(), history=4):
        """Figure out where GN should be at the given date.

        By default, consider the last 4 to give preference to people who
        haven't been hosting."""

        if type(when) != datetime.date:
            when = when.date()

        logging.info('Resolving gamenight for %s', when)

        invitations = cls.query(cls.date == when)
        logging.debug('Query: %r' % invitations)

        candidates = []
        # check each level separately
        for pri in ('Insist', 'Want', 'Can'):
            candidates = dict([(x.owner.id(), x) for x in
                    invitations.filter(cls.priority == pri).fetch()])

            # no matches at this priority
            if not candidates:
                logging.debug('none at priority %s' % pri)
                continue

            # no need to look at lower levels
            logging.debug('Candidates(%s): %r' % (pri, candidates))
            break

        # no one wants to host :(
        if not candidates:
            logging.debug('none found.')
            return None

        # more than one option, filter out the recent hosts until we run out of
        # recent hosts, or have just one option left.
        logging.debug('Candidates: %r' % candidates.keys())
        if len(candidates) > 1:
            if history:
                old_nights = Gamenight.query(Gamenight.date < Utils.now()).\
                                 order(Gamenight.date).fetch(history)
                while old_nights and len(candidates) > 1:
                    owner = old_nights.pop().owner.id()
                    if owner in candidates:
                        logging.debug('removing previous host %s' % owner)
                        del candidates[owner]

        logging.debug('Not recent candidates: %r' % candidates.keys())

        # pick one at random, return the invitation object
        selected = random.choice(candidates.keys())
        logging.debug('Selected invitation: %s\n%r' %
                      (selected, candidates[selected]))

        return candidates[selected].make_gamenight()

    @classmethod
    def summary(cls):
        """Get upcoming saturdays, and who has invited.

        Returns a dictionary of dates, with a list of (who, priority) for each.
        """

        invitations = cls.query(cls.date >= Utils.now()).\
                          filter(cls.date < Utils.now() +
                                 datetime.timedelta(weeks=8)).\
                          order(cls.date)

        res = {}
        invlist = defaultdict(list)
        for invite in invitations.iter():
            invlist[invite.date].append(invite.owner.get().name)

        for date, invites in invlist.iteritems():
            res[date] = ', '.join(name for name in invites)

        return res

    @classmethod
    def create(cls, args):
        """Create or update an invitation.

        Any given owner can have just invite per date."""

        invite = Invitation.query(Invitation.date == args['when']).\
                            filter(Invitation.owner == args['owner']).get()

        if invite:
            invite.location = args['where']
            invite.notes = args['notes']
            invite.priority = args['priority']
            updated = True
        else:
            invite = Invitation(date = args['when'].date(),
                                time = args['when'].time(),
                                owner = args['owner'],
                                location = args['where'],
                                notes = args['notes'],
                                priority = args['priority'],
                                parent = Invitation.dummy(),
                               )
            updated = False
        ndb.transaction(lambda: invite.put())

        return updated, invite

    def make_gamenight(self, overwrite=False):
        """Create an unsaved gamenight object from an invitation.

        Args:
            overwrite - if an existing GN is already scheduled, replace it. If
                        false, return it unchanged.
        """

        gamenight = Gamenight.query(Gamenight.date==self.date).get()

        if gamenight and not overwrite:
            if gamenight.status == 'Yes':
                return gamenight

        if not gamenight:
            gamenight = Gamenight(date=self.date)

        gamenight.invitation = self.key
        gamenight.status = 'Yes'
        gamenight.lastupdate = Utils.now()
        gamenight.time = self.time
        gamenight.owner = self.owner
        gamenight.location = self.location
        gamenight.notes = self.notes

        return gamenight

class User(ndb.Model):
    """Accounts of people who host."""
    location = ndb.StringProperty('l', indexed=False)
    color = ndb.StringProperty('c', indexed=False)
    superuser = ndb.BooleanProperty('s')
    nag = ndb.BooleanProperty('e')
    name = ndb.StringProperty('n')

    @classmethod
    def lookup(cls, key):
        return ndb.Key(cls, key).get()

    @classmethod
    def get(cls, userobj):
        user = ndb.Key(cls, userobj.email()).get()
        if user is None:
            user = cls(id=userobj.email())
            user.name = userobj.nickname()
            if user.name in [None, 'None', ''] or user.name.find("http://") > -1:
                animals = [ 'bear', 'emu', 'zebu', 'snake', 'bird', 'awk',
                            'quahog', 'rutabaga', 'rabbit', 'dragon', 'boar',
                            'horse', 'crab', 'fish', 'libra', 'alicorn',
                            'moose', 'geoduck', 'Nudibranch' ]
                user.name = 'Some %s' % random.choice(animals).title()

            user.put()

        return user

class Config(ndb.Model):
    """Store application configuration values."""
    name = ndb.StringProperty('n')
    value = ndb.StringProperty('v')

    @classmethod
    def update(cls, key, val):
        c = cls.query(cls.name==key).get()
        if not c:
            logging.warning('Trying to update an unknown key %s: %s', key, val)
            return None

        if c.value == val:
            return False

        c.value = val
        c.put()
        return True

class Auth(ndb.Model):
    """Store approximately one record, the oauth token for calendar access."""
    credentials = CredentialsNDBProperty('c')

    @classmethod
    def get(cls):
        creds = cls.query().get()
        if creds:
            return creds.credentials
        else:
            return None

# vim: set ts=4 sts=4 sw=4 et:
