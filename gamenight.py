import os
import urllib

from datetime import datetime

from google.appengine.api import users
from google.appengine.ext import ndb

import jinja2
import webapp2

JINJA_ENVIRONMNT = jinja2.Environment(
  loader=jinja2.FileSystemLoader(os.path.dirname(__file__)),
  extensions=['jinja2.ext.autoescape'])

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


class MainPage(webapp2.RequestHandler):

    def get(self):
        futurenights = Gamenight.future(10)
        gamenight = GamenightNext.query().fetch(1)[0]
        template_values = {
          'future': futurenights,
          'status': gamenight.status,
          'where': gamenight.location,
          'when': gamenight.time.strftime('%I:%M %p'),
          'updated': gamenight.lastupdate.strftime('%A, %B %d, %I:%M %p'),
        }

        template = JINJA_ENVIRONMNT.get_template('index.html')
        # Write the submission form and the footer of the page
        self.response.write(template.render(template_values))


application = webapp2.WSGIApplication([
    ('/', MainPage),
], debug=True)

