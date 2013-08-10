import logging
import os
import urllib

from google.appengine.api import users

import jinja2
import webapp2


from schema import Gamenight, Application, User, Pacific_tzinfo

from datetime import datetime

JINJA_ENVIRONMNT = jinja2.Environment(
  loader=jinja2.FileSystemLoader(os.path.dirname(__file__)),
  extensions=['jinja2.ext.autoescape'])

PT = Pacific_tzinfo()

class MainPage(webapp2.RequestHandler):

    def get(self):
        futurenights = Gamenight.future(10)

        if futurenights and futurenights[0].this_week():
            # we have a gamenight scheduled for this week
            gamenight = futurenights[0]
            if len(futurenights) > 1:
                futurenights = futurenights[1:]
            else:
                futurenights = None
        else:
            gamenight = Gamenight(status='No',
                                  date=Utils.Saturday(),
                                  lastupdate=Utils.Now())
            gamenight.put()

        updated = gamenight.lastupdate.strftime('%A, %B %d, %I:%M %p')
        template_values = {
          'future': futurenights,
          'status': gamenight.status,
          'updated': updated,
        }

        if gamenight.date:
            template_values['when'] = gamenight.date.strftime('%I:%M %p')

        if gamenight.location:
            template_values['where'] =  gamenight.location

        template = JINJA_ENVIRONMNT.get_template('index.html')
        # Write the submission form and the footer of the page
        self.response.write(template.render(template_values))


class EditPage(webapp2.RequestHandler):

    def get(self):

        sys_user = users.get_current_user()
        if not sys_user:
            self.redirect(users.create_login_url(self.request.uri))
            return

        user = User.get_or_insert(sys_user.email())

        futurenights = Gamenight.future(100)
        template_values = {
            'scheduled': futurenights,
            'logout': users.create_logout_url('/'),
            'user': user.key.id(),
            'admin': user.superuser,
            'users': User.query().fetch(100),
        }

        template = JINJA_ENVIRONMNT.get_template('edit.html')
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

    @classmethod
    def Saturday(cls):
        d = datetime.today().replace(hour=20, minute=0, second=0)
        weekday = d.weekday()
        if weekday == 6:
            return d + timedelta(days=6)
        elif weekday < 5:
            return d + timedelta(days=5-weekday)
        else:
            return d

    @classmethod
    def Now(cls):
        now = datetime.now()
        return now + PT.utcoffset(now)

application = webapp2.WSGIApplication([
    ('/', MainPage),
    ('/edit', EditPage),
], debug=True)

# vim: set ts=4 sts=4 sw=4 et:

