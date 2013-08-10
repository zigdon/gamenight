from google.appengine.ext import ndb

from datetime import timedelta, date
from utils import Utils

class Gamenight(ndb.Model):
    """Gamenights that have been scheduled."""
    application = ndb.KeyProperty('a', kind='Application')
    event = ndb.StringProperty('e')
    status = ndb.StringProperty('s', choices=['Yes', 'Probably', 'Maybe', 'No'])
    lastupdate = ndb.DateTimeProperty('u')

    # denormalized from Application for datastore efficiency
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


class Application(ndb.Model):
    """Entries of offers to host."""
    date = ndb.DateTimeProperty('d')
    owner = ndb.KeyProperty('o', kind='User')
    location = ndb.StringProperty('l', indexed=False)
    notes = ndb.StringProperty('n', indexed=False)
    priority = ndb.StringProperty('p', choices=['Can', 'Want', 'Need'])


class User(ndb.Model):
    """Accounts of people who host."""
    name = ndb.StringProperty('n')
    location = ndb.StringProperty('l', indexed=False)
    color = ndb.StringProperty('c', indexed=False)
    superuser = ndb.BooleanProperty('s')


# vim: set ts=4 sts=4 sw=4 et:
