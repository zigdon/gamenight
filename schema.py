from google.appengine.ext import ndb

from datetime import datetime

class GamenightNext(ndb.Model):
    """Coming week's gamenight details."""
    gamenight = ndb.KeyProperty('g', kind='Gamenight')
    status = ndb.StringProperty('s', choices=['yes', 'probably', 'no'])
    time = ndb.TimeProperty('t')
    location = ndb.StringProperty('l', indexed=False)
    notes = ndb.StringProperty('n', indexed=False)
    lastupdate = ndb.DateTimeProperty('u')


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


class Application(ndb.Model):
    """Entries of offers to host."""
    date = ndb.DateTimeProperty('d')
    owner = ndb.KeyProperty('o', kind='User')
    location = ndb.StringProperty('l', indexed=False)
    notes = ndb.StringProperty('n', indexed=False)
    priority = ndb.StringProperty('p', choices=['can', 'want', 'need'])


class User(ndb.Model):
    """Accounts of people who host."""
    account = ndb.UserProperty('a')
    location = ndb.StringProperty('l', indexed=False)
    color = ndb.StringProperty('c', indexed=False)
    superuser = ndb.BooleanProperty('s')

# vim: set ts=4 sts=4 sw=4 et:
