import logging
import os
import urllib

from google.appengine.api import users

import jinja2
import webapp2


from schema import GamenightNext, Gamenight, Application, User

from datetime import datetime

JINJA_ENVIRONMNT = jinja2.Environment(
  loader=jinja2.FileSystemLoader(os.path.dirname(__file__)),
  extensions=['jinja2.ext.autoescape'])


class MainPage(webapp2.RequestHandler):

    def get(self):
        futurenights = Gamenight.future(10)
        gamenight = GamenightNext.get()
        template_values = {
          'future': futurenights,
          'status': gamenight.status,
          'updated': gamenight.lastupdate.strftime('%A, %B %d, %I:%M %p'),
        }

        if gamenight.date:
            template_values['when'] = gamenight.date.strftime('%I:%M %p')

        if gamenight.location:
            template_values['where'] =  gamenight.location

        template = JINJA_ENVIRONMNT.get_template('index.html')
        # Write the submission form and the footer of the page
        self.response.write(template.render(template_values))


class Utils:

    @classmethod
    def scheduleGamenight(cls):
        gn_schedule = Gamenight.this_week()
        gn_next = GamenightNext.get()

        # none scheduled, or at least not for this wee
        if not gn_schedule:
            if gn_next:
                gn_next.populate(status='Probably', location=None, date=None)
            else:
                gn_next = GamenightNext(status='Probably')

            gn_next.put()
            return

        gn_next.populate(status='Yes',
                         location=gn_schedule.location,
                         date=gn_schedule.date,
                         notes=gn_schedule.notes)
        gn_next.put()


application = webapp2.WSGIApplication([
    ('/', MainPage),
], debug=True)

# vim: set ts=4 sts=4 sw=4 et:

