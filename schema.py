from google.appengine.ext import ndb

from datetime import datetime, timedelta, date, time

class GamenightNext(ndb.Model):
    """Coming week's gamenight details."""
    gamenight = ndb.KeyProperty('g', kind='Gamenight')
    status = ndb.StringProperty('s', choices=['Yes', 'Probably', 'No'])
    date = ndb.DateTimeProperty('d')
    location = ndb.StringProperty('l', indexed=False)
    notes = ndb.StringProperty('n', indexed=False)
    lastupdate = ndb.DateTimeProperty('u', auto_now=True)

    @classmethod
    def get(cls):
        gamenight = cls.query().fetch(1)
        if gamenight:
            return gamenight[0]
        else:
            return None



class Gamenight(ndb.Model):
    """Gamenights that have been scheduled."""
    application = ndb.KeyProperty('a', kind='Application')
    event = ndb.StringProperty('e')
    status = ndb.StringProperty('s', choices=['yes', 'probably', 'no'])

    # denormalized from Application for datastore efficiency
    date = ndb.DateTimeProperty('d')
    owner = ndb.KeyProperty('o', kind='User')
    location = ndb.StringProperty('l', indexed=False)
    notes = ndb.StringProperty('n', indexed=False)
    priority = ndb.StringProperty('p', choices=['can', 'want', 'need'])
    status = ndb.StringProperty('s', choices=['yes', 'probably', 'no'])

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
    priority = ndb.StringProperty('p', choices=['can', 'want', 'need'])


class User(ndb.Model):
    """Accounts of people who host."""
    location = ndb.StringProperty('l', indexed=False)
    color = ndb.StringProperty('c', indexed=False)
    superuser = ndb.BooleanProperty('s')

# vim: set ts=4 sts=4 sw=4 et:
