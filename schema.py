from google.appengine.ext import ndb

from datetime import datetime, timedelta, date, time, tzinfo

class Gamenight(ndb.Model):
    """Gamenights that have been scheduled."""
    application = ndb.KeyProperty('a', kind='Application')
    event = ndb.StringProperty('e')
    status = ndb.StringProperty('s', choices=['Yes', 'Probably', 'No'])
    lastupdate = ndb.DateTimeProperty('u')

    # denormalized from Application for datastore efficiency
    date = ndb.DateTimeProperty('d')
    owner = ndb.KeyProperty('o', kind='User')
    location = ndb.StringProperty('l', indexed=False)
    notes = ndb.StringProperty('n', indexed=False)
    priority = ndb.StringProperty('p', choices=['Can', 'Want', 'Insist'])

    @classmethod
    def future(cls, limit):
        return cls.query(cls.date > datetime.now()).order(cls.date).fetch(limit)

    @classmethod
    def this_week(cls):
        """Get this week's gamenight."""

        g = cls.future(1)
        if g and g[0].is_this_week():
            return g[0]
        else:
            return None

    def is_this_week(self):
        sat = date.today()
        if sat.weekday() < 5: # mon-fri
            sat += timedelta(5 - sat.weekday())
        elif sat.weekday() > 5: # sun
            sat += timedelta(6)

        return sat - self.date.date() == timedelta(0)


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


class Pacific_tzinfo(tzinfo):
    """Implementation of the Pacific timezone."""
    def utcoffset(self, dt):
        return timedelta(hours=-8) + self.dst(dt)

    def _FirstSunday(self, dt):
        """First Sunday on or after dt."""
        return dt + timedelta(days=(6-dt.weekday()))

    def dst(self, dt):
        # 2 am on the second Sunday in March
        dst_start = self._FirstSunday(datetime(dt.year, 3, 8, 2))
        # 1 am on the first Sunday in November
        dst_end = self._FirstSunday(datetime(dt.year, 11, 1, 1))

        if dst_start <= dt.replace(tzinfo=None) < dst_end:
            return timedelta(hours=1)
        else:
            return timedelta(hours=0)

    def tzname(self, dt):
        if self.dst(dt) == timedelta(hours=0):
            return "PST"
        else:
            return "PDT"

# vim: set ts=4 sts=4 sw=4 et:
