from collections import defaultdict
from datetime import timedelta, date
from utils import Utils

from google.appengine.ext import ndb

class Gamenight(ndb.Model):
    """Gamenights that have been scheduled."""
    invitation = ndb.KeyProperty('a', kind='Invitation')
    event = ndb.StringProperty('e')
    status = ndb.StringProperty('s', choices=['Yes', 'Probably', 'Maybe', 'No'])
    lastupdate = ndb.DateTimeProperty('u')

    # denormalized from invitation for datastore efficiency
    date = ndb.DateTimeProperty('d')
    owner = ndb.KeyProperty('o', kind='User')
    location = ndb.StringProperty('l', indexed=False)
    notes = ndb.StringProperty('n', indexed=False)
    priority = ndb.StringProperty('p', choices=['Can', 'Want', 'Insist'])

    @classmethod
    def schedule(cls):
        schedule = cls.future(1)
        if not schedule:
            schedule = Gamenight(status='Maybe',
                                 date=Utils.Saturday(),
                                 lastupdate=Utils.Now())
            schedule.put()
        return schedule

    @classmethod
    def future(cls, limit):
        return cls.query(cls.date >= Utils.Now()).order(cls.date).fetch(limit)

    @classmethod
    def this_week(cls):
        """Get this week's gamenight."""

        g = cls.future(1)
        if g and g[0].is_this_week():
            return g[0]
        else:
            return None

    def is_this_week(self):
        return self.date.date() - date.today() < timedelta(7)


class Invitation(ndb.Model):
    """Entries of offers to host."""
    date = ndb.DateTimeProperty('d')
    owner = ndb.KeyProperty('o', kind='User')
    location = ndb.StringProperty('l', indexed=False)
    notes = ndb.StringProperty('n', indexed=False)
    priority = ndb.StringProperty('p', choices=['Can', 'Want', 'Insist'])

    @classmethod
    def summary(cls):
        """Get upcoming saturdays, and who has invited."""

        invitations = cls.query(cls.date >= Utils.Now()).\
                          filter(cls.date < Utils.Now() + timedelta(weeks=8)).\
                          order(cls.date)

        res = defaultdict(list)
        for invite in invitations.iter():
            res[invite.date.date()].append("%s (%s)" %
                (invite.owner.get().name, invite.priority))

        for date in res.keys():
            res[date] = ", ".join(res[date])

        return res

    @classmethod
    def create(cls, args):
        invite = Invitation.query(Invitation.date == args['when']).\
                            filter(Invitation.owner == args['owner']).get()

        if invite:
            invite.location = args['where']
            invite.notes = args['notes']
            invite.priority = args['priority']
        else:
            invite = Invitation(date = args['when'],
                                owner = args['owner'],
                                location = args['where'],
                                notes = args['notes'],
                                priority = args['priority'],
                               )
        invite.put()

        return invite

class User(ndb.Model):
    """Accounts of people who host."""
    name = ndb.StringProperty('n')
    location = ndb.StringProperty('l', indexed=False)
    color = ndb.StringProperty('c', indexed=False)
    superuser = ndb.BooleanProperty('s')


# vim: set ts=4 sts=4 sw=4 et:
